package webrtc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
	"omnicast/internal/broker"
	"omnicast/internal/models"
)

// DefaultCascadeRTCConfiguration provides default STUN configuration for inter-server connections
var DefaultCascadeRTCConfiguration = webrtc.Configuration{
	ICEServers: []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	},
}

// CascadeSession manages an active inter-server WebRTC track relay connection between Edge and Origin
type CascadeSession struct {
	RoomID         string
	OriginAddr     string
	PeerConnection *webrtc.PeerConnection
	WSConn         *websocket.Conn
	ctx            context.Context
	cancel         context.CancelFunc
	closed         bool
	mu             sync.Mutex
}

// Close gracefully closes the cascading session and tears down WebRTC and WebSocket connections
func (cs *CascadeSession) Close() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.closed {
		return
	}
	cs.closed = true
	cs.cancel()

	if cs.PeerConnection != nil {
		_ = cs.PeerConnection.Close()
	}
	if cs.WSConn != nil {
		_ = cs.WSConn.Close()
	}
	log.Printf("[SFU Cascade] Closed inter-server session for Room: %s\n", cs.RoomID)
}

// CascadeManager manages all inter-server WebRTC pull sessions for Edge nodes
type CascadeManager struct {
	webrtcAPI *webrtc.API
	broker    *broker.RedisBroker
	serverID  string
	sessions  map[string]*CascadeSession
	mu        sync.RWMutex
}

// NewCascadeManager creates a new CascadeManager instance for SFU Cascading
func NewCascadeManager(webrtcAPI *webrtc.API, broker *broker.RedisBroker, serverID string) *CascadeManager {
	if serverID == "" {
		serverID = "edge-node-1"
	}
	return &CascadeManager{
		webrtcAPI: webrtcAPI,
		broker:    broker,
		serverID:  serverID,
		sessions:  make(map[string]*CascadeSession),
	}
}

// SetBroker attaches or updates the RedisBroker instance
func (cm *CascadeManager) SetBroker(b *broker.RedisBroker) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.broker = b
}

// EnsureCascaded checks if the room has local media tracks, and if not, pulls them from the Origin server
func (cm *CascadeManager) EnsureCascaded(room *models.Room) error {
	if room == nil {
		return errors.New("room is nil")
	}

	// 1. If local room already has active video tracks, no need to cascade
	if room.GetVideoTrack() != nil || room.GetDefaultViewerVideoTrack() != nil {
		return nil
	}

	cm.mu.Lock()
	if _, exists := cm.sessions[room.RoomID]; exists {
		cm.mu.Unlock()
		return nil // Cascade session already established or connecting
	}

	if cm.broker == nil || !cm.broker.IsActive() {
		cm.mu.Unlock()
		return errors.New("cannot cascade: Redis global room registry is not active")
	}

	// 2. Lookup Origin server address for this room from Redis
	originAddr, err := cm.broker.GetRoomOrigin(room.RoomID)
	if err != nil || originAddr == "" {
		cm.mu.Unlock()
		return fmt.Errorf("failed to locate origin server for room %s: %w", room.RoomID, err)
	}

	log.Printf("[SFU Cascade] Initiating inter-server WebRTC cascade for Room '%s' from Origin: %s\n", room.RoomID, originAddr)

	ctx, cancel := context.WithCancel(context.Background())
	session := &CascadeSession{
		RoomID:     room.RoomID,
		OriginAddr: originAddr,
		ctx:        ctx,
		cancel:     cancel,
	}
	cm.sessions[room.RoomID] = session
	cm.mu.Unlock()

	// 3. Connect to Origin server and pull tracks asynchronously
	go func() {
		if err := cm.connectAndRelay(session, room); err != nil {
			log.Printf("[SFU Cascade Error] Failed to cascade room %s from origin %s: %v\n", room.RoomID, originAddr, err)
			cm.CloseSession(room.RoomID)
		}
	}()

	return nil
}

// connectAndRelay connects via WebSocket to Origin, sets up RTCPeerConnection, and relays RTP packets to local Room
func (cm *CascadeManager) connectAndRelay(session *CascadeSession, room *models.Room) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	serverSecret := os.Getenv("SERVER_SECRET")
	if serverSecret == "" {
		serverSecret = "super_secret_cascade_key_123"
	}

	dialURL := session.OriginAddr
	if strings.Contains(dialURL, "?") {
		dialURL += "&server_secret=" + url.QueryEscape(serverSecret) + "&server_id=" + url.QueryEscape(cm.serverID)
	} else {
		dialURL += "?server_secret=" + url.QueryEscape(serverSecret) + "&server_id=" + url.QueryEscape(cm.serverID)
	}

	header := http.Header{}
	header.Add("Sec-WebSocket-Protocol", "media-signaling")
	header.Add("X-Server-Secret", serverSecret)

	wsConn, _, err := dialer.Dial(dialURL, header)
	if err != nil {
		return fmt.Errorf("failed to dial origin websocket (%s): %w", dialURL, err)
	}
	session.WSConn = wsConn

	// Create Inter-Server PeerConnection with dynamic TURN/STUN configuration
	pc, err := cm.webrtcAPI.NewPeerConnection(GetDynamicRTCConfiguration("cascade-edge-" + cm.serverID))
	if err != nil {
		return fmt.Errorf("failed to create inter-server PeerConnection: %w", err)
	}
	session.PeerConnection = pc

	// Add recvonly transceivers for Audio and Video
	if _, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	}); err != nil {
		return err
	}
	if _, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	}); err != nil {
		return err
	}

	// OnTrack: receive remote RTP tracks from Origin and convert to local Room tracks
	pc.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		rid := remoteTrack.RID()
		log.Printf("[SFU Cascade] Received remote track from Origin: Kind=%s, RID=%s, SSRC=%d\n",
			remoteTrack.Kind(), rid, remoteTrack.SSRC())

		localTrack, trackErr := webrtc.NewTrackLocalStaticRTP(
			remoteTrack.Codec().RTPCodecCapability,
			remoteTrack.ID(),
			remoteTrack.StreamID(),
		)
		if trackErr != nil {
			log.Printf("[SFU Cascade Error] Failed to create local relayed track: %v\n", trackErr)
			return
		}

		switch remoteTrack.Kind() {
		case webrtc.RTPCodecTypeVideo:
			if rid == "" {
				room.SetVideoTrack(localTrack)
				log.Printf("[SFU Cascade] Set default video track on Edge node for Room: %s\n", room.RoomID)
			} else {
				room.SetVideoTrackRID(rid, localTrack)
				log.Printf("[SFU Cascade] Set simulcast video layer ('%s') on Edge node for Room: %s\n", rid, room.RoomID)
			}
			room.SetHostVideoSSRC(uint32(remoteTrack.SSRC()))

			// Send periodic PLI keyframe requests to Origin
			go func() {
				ticker := time.NewTicker(3 * time.Second)
				defer ticker.Stop()
				for range ticker.C {
					if pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
						return
					}
					_ = pc.WriteRTCP([]rtcp.Packet{
						&rtcp.PictureLossIndication{MediaSSRC: uint32(remoteTrack.SSRC())},
					})
				}
			}()

		case webrtc.RTPCodecTypeAudio:
			room.SetAudioTrack(localTrack)
			log.Printf("[SFU Cascade] Set audio track on Edge node for Room: %s\n", room.RoomID)
		}

		// Relay loop: Read RTP packets from Origin and write to Edge's local TrackLocalStaticRTP (Zero-Allocation)
		go func() {
			bufPtr := GetRTPBuffer()
			defer PutRTPBuffer(bufPtr)
			buf := *bufPtr

			for {
				n, _, readErr := remoteTrack.Read(buf)
				if readErr != nil {
					if !errors.Is(readErr, io.EOF) {
						log.Printf("[SFU Cascade] Stream read ended from Origin (%s): %v\n", room.RoomID, readErr)
					}
					return
				}
				if _, writeErr := localTrack.Write(buf[:n]); writeErr != nil {
					// Local viewers may have closed
				}
			}
		}()
	})

	// On ICE Candidate from Edge -> Send to Origin
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			candJSON := candidate.ToJSON()
			candBytes, _ := json.Marshal(candJSON)
			iceMsg := models.SignalingMessage{
				Event:   "ice",
				RoomID:  room.RoomID,
				UserID:  "edge-" + cm.serverID,
				Payload: candBytes,
			}
			iceBytes, _ := iceMsg.Encode()
			_ = wsConn.WriteMessage(websocket.TextMessage, iceBytes)
		}
	})

	// Create Offer and send join_room to Origin
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("failed to create cascade offer: %w", err)
	}
	if err = pc.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("failed to set cascade local description: %w", err)
	}

	offerBytes, _ := json.Marshal(offer)
	joinMsg := models.SignalingMessage{
		Event:   "join_room",
		RoomID:  room.RoomID,
		UserID:  "edge-" + cm.serverID,
		Payload: offerBytes,
	}
	joinBytes, err := joinMsg.Encode()
	if err != nil {
		return err
	}

	if err = wsConn.WriteMessage(websocket.TextMessage, joinBytes); err != nil {
		return fmt.Errorf("failed to send join_room to origin: %w", err)
	}

	// Read loop from Origin signaling WebSocket
	for {
		_, msgBytes, err := wsConn.ReadMessage()
		if err != nil {
			return fmt.Errorf("origin websocket disconnected: %w", err)
		}

		var sigMsg models.SignalingMessage
		if err := json.Unmarshal(msgBytes, &sigMsg); err != nil {
			continue
		}

		switch sigMsg.Event {
		case "answer":
			var answer webrtc.SessionDescription
			answerBytes, _ := json.Marshal(sigMsg.Payload)
			if err := json.Unmarshal(answerBytes, &answer); err == nil {
				if err := pc.SetRemoteDescription(answer); err != nil {
					log.Printf("[SFU Cascade Error] Failed to set remote description from origin: %v\n", err)
				} else {
					log.Printf("[SFU Cascade Success] Connected to Origin for Room: %s\n", room.RoomID)
				}
			}

		case "ice", "candidate":
			var cand webrtc.ICECandidateInit
			candBytes, _ := json.Marshal(sigMsg.Payload)
			if err := json.Unmarshal(candBytes, &cand); err == nil {
				_ = pc.AddICECandidate(cand)
			}

		case "room_closed":
			log.Printf("[SFU Cascade] Origin notified room_closed for Room: %s\n", room.RoomID)
			cm.CloseSession(room.RoomID)
			return nil
		}
	}
}

// CloseSession terminates a specific room cascade session
func (cm *CascadeManager) CloseSession(roomID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if session, exists := cm.sessions[roomID]; exists && session != nil {
		session.Close()
		delete(cm.sessions, roomID)
	}
}

// Close terminates all cascade sessions
func (cm *CascadeManager) Close() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for roomID, session := range cm.sessions {
		if session != nil {
			session.Close()
		}
		delete(cm.sessions, roomID)
	}
}
