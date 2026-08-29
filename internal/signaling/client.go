package signaling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"omnicast/internal/api"
	"omnicast/internal/config"
	"omnicast/internal/models"
	internalWebRTC "omnicast/internal/webrtc"

	"github.com/gofiber/contrib/websocket"
	"github.com/google/uuid"
	"github.com/pion/webrtc/v3"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer (15 seconds before zombie cleanup).
	pongWait = 15 * time.Second

	// Send pings to peer every 5 seconds.
	pingPeriod = 5 * time.Second

	// Maximum message size allowed from peer (512 KB).
	maxMessageSize = 512 * 1024
)

// DefaultRTCConfiguration sets default ICE servers (Google STUN)
var DefaultRTCConfiguration = webrtc.Configuration{
	ICEServers: []webrtc.ICEServer{
		{
			URLs: []string{
				"stun:stun.l.google.com:19302",
				"stun:stun1.l.google.com:19302",
			},
		},
	},
}

// Client represents a connected WebSocket client
type Client struct {
	ID               string                 `json:"id"`
	UserName         string                 `json:"user_name,omitempty"`
	AvatarURL        string                 `json:"avatar_url,omitempty"`
	Role             string                 `json:"role,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	RoomID           string                 `json:"room_id,omitempty"`
	Claims           *UserClaims            `json:"-"`
	Hub              *Hub                   `json:"-"`
	RoomManager      *RoomManager           `json:"-"`
	WebRTCAPI        *webrtc.API            `json:"-"`
	PeerConnection   *webrtc.PeerConnection `json:"-"`
	Conn             *websocket.Conn        `json:"-"`
	Send             chan []byte            `json:"-"`
	lastPongReceived atomic.Int64           `json:"-"`
	closed           bool
	writeMu          sync.Mutex
	mu               sync.Mutex
}

// UpdateLastPong updates the lastPongReceived timestamp to current unix time
func (c *Client) UpdateLastPong() {
	c.lastPongReceived.Store(time.Now().Unix())
}

// GetLastPong returns the last pong timestamp in unix seconds
func (c *Client) GetLastPong() int64 {
	return c.lastPongReceived.Load()
}

// NewClient creates and initializes a new Client with a generated UUID
func NewClient(hub *Hub, roomManager *RoomManager, api *webrtc.API, conn *websocket.Conn) *Client {
	c := &Client{
		ID:          uuid.New().String(),
		Hub:         hub,
		RoomManager: roomManager,
		WebRTCAPI:   api,
		Conn:        conn,
		Send:        make(chan []byte, 256),
	}
	c.lastPongReceived.Store(time.Now().Unix())
	return c
}

// NewClientWithClaims creates a Client initialized with authenticated JWT claims
func NewClientWithClaims(hub *Hub, roomManager *RoomManager, api *webrtc.API, conn *websocket.Conn, claims *UserClaims) *Client {
	clientID := uuid.New().String()
	userName := ""
	avatarURL := ""
	role := "viewer"
	roomID := ""
	var metadata map[string]interface{}

	if claims != nil {
		if claims.UserID != "" {
			clientID = claims.UserID
		}
		if claims.DisplayName != "" {
			userName = claims.DisplayName
		} else if claims.UserName != "" {
			userName = claims.UserName
		}
		if claims.AvatarURL != "" {
			avatarURL = claims.AvatarURL
		}
		if claims.Role != "" {
			role = claims.Role
		}
		if claims.RoomID != "" {
			roomID = claims.RoomID
		}
		if claims.Metadata != nil {
			metadata = claims.Metadata
		}
	}

	c := &Client{
		ID:          clientID,
		UserName:    userName,
		AvatarURL:   avatarURL,
		Role:        role,
		Metadata:    metadata,
		RoomID:      roomID,
		Claims:      claims,
		Hub:         hub,
		RoomManager: roomManager,
		WebRTCAPI:   api,
		Conn:        conn,
		Send:        make(chan []byte, 256),
	}
	c.lastPongReceived.Store(time.Now().Unix())
	return c
}

// ToParticipant converts client profile metadata to a models.Participant struct
func (c *Client) ToParticipant() *models.Participant {
	c.mu.Lock()
	defer c.mu.Unlock()
	displayName := c.UserName
	if displayName == "" {
		displayName = c.ID
	}
	var metaCopy map[string]interface{}
	if c.Metadata != nil {
		metaCopy = make(map[string]interface{}, len(c.Metadata))
		for k, v := range c.Metadata {
			metaCopy[k] = v
		}
	}
	return &models.Participant{
		UserID:      c.ID,
		DisplayName: displayName,
		AvatarURL:   c.AvatarURL,
		Role:        c.Role,
		Metadata:    metaCopy,
		JoinedAt:    time.Now().UTC(),
	}
}

// ReadPump pumps messages from the websocket connection, parses signaling messages,
// and routes WebRTC offer/answer and ICE candidate events.
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.unregister <- c
		c.mu.Lock()
		if c.PeerConnection != nil {
			_ = c.PeerConnection.Close()
		}
		c.mu.Unlock()
		_ = c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	_ = c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.UpdateLastPong()
	c.Conn.SetPongHandler(func(string) error {
		c.UpdateLastPong()
		_ = c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, rawMsg, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket unexpected close error (client %s): %v\n", c.ID, err)
			}
			break
		}

		c.UpdateLastPong()

		// Parse signaling message JSON
		sigMsg, parseErr := models.ParseSignalingMessage(rawMsg)
		if parseErr != nil {
			log.Printf("Invalid signaling message from client %s: %v\n", c.ID, parseErr)
			continue
		}

		c.handleSignalingMessage(sigMsg)
	}
}

// handleSignalingMessage routes WebRTC signaling events (publish/offer, play/join_room, chat, ice candidate, etc.)
func (c *Client) handleSignalingMessage(msg *models.SignalingMessage) {
	switch msg.Event {
	case "ping":
		c.UpdateLastPong()
		pongPayload, _ := json.Marshal(map[string]any{"timestamp": time.Now().Unix()})
		if enc, err := (&models.SignalingMessage{
			Event:   "pong",
			RoomID:  msg.RoomID,
			Payload: pongPayload,
		}).Encode(); err == nil {
			c.SafeSend(enc)
		}

	case "pong":
		c.UpdateLastPong()

	case "create_room", "publish", "offer":
		c.handlePublishOffer(msg)

	case "play", "join_room", "join", "subscribe":
		c.handleViewerJoinPlay(msg)

	case "chat", "chat_message":
		c.handleChatMessage(msg)

	case "gift", "send_gift", "gift_sent":
		c.handleGiftMessage(msg)

	case "seat_request", "request_seat":
		c.handleSeatRequest(msg)

	case "seat_accept", "accept_seat":
		c.handleSeatAccept(msg)

	case "leave_seat", "seat_leave":
		c.handleLeaveSeat(msg)

	case "kick_seat", "seat_kick":
		c.handleKickSeat(msg)

	case "kick_participant", "kick_user":
		c.handleKickParticipant(msg)

	case "end_room", "close_room", "stop_room":
		c.handleEndRoom(msg)

	case "pk_request", "request_pk":
		c.handlePKRequest(msg)

	case "pk_accept", "accept_pk", "pk_start", "start_pk":
		c.handlePKAccept(msg)

	case "pk_stop", "stop_pk", "end_pk", "pk_end":
		c.handlePKStop(msg)

	case "subscribe_cohost", "subscribe_co_host", "play_cohost":
		c.handleSubscribeCoHost(msg)

	case "media_state", "media_state_change", "set_media_state", "track_muted", "track_unmuted", "mute_track", "mute":
		c.handleMediaStateChange(msg)

	case "sync_state", "room_info", "room_state":
		c.handleRoomStateSync(msg)

	case "ice", "candidate":
		c.handleICECandidate(msg)

	case "ice_restart", "restart_ice", "renegotiate_ice":
		c.handleICERestartRequest(msg)

	case "answer", "sdp_answer":
		c.handleSDPAnswer(msg)

	case "request_layer", "select_layer", "set_layer", "switch_layer":
		c.handleRequestLayer(msg)

	case "set_viewport", "update_viewport", "viewport":
		c.handleSetViewport(msg)

	default:
		log.Printf("Unhandled signaling event '%s' from client %s\n", msg.Event, c.ID)
	}
}

// handlePublishOffer handles SDP offer from a host or co-host wanting to broadcast media
func (c *Client) handlePublishOffer(msg *models.SignalingMessage) {
	if c.WebRTCAPI == nil {
		log.Printf("WebRTC API not initialized for client %s\n", c.ID)
		return
	}

	roomID := msg.RoomID
	if roomID == "" {
		roomID = "default-room"
	}

	// Access Control: If a user tries to publish a track (Host/Co-host) but their token claim says can_publish: false, forcefully reject the WebRTC offer.
	if c.Claims != nil && !c.Claims.AllowsPublishing() {
		log.Printf("[Access Control] Forcefully rejected publish offer from client %s (can_publish=false or insufficient role '%s')\n", c.ID, c.Claims.Role)
		errPayload, _ := json.Marshal(map[string]any{
			"status_code": 403,
			"error":       "Forbidden: token claims do not permit publishing (can_publish: false)",
		})
		if enc, err := (&models.SignalingMessage{
			Event:   "error",
			RoomID:  roomID,
			Payload: errPayload,
		}).Encode(); err == nil {
			c.SafeSend(enc)
		}
		return
	}

	// Extract display name, avatar URL, and dynamic metadata if provided
	if msg.DisplayName != "" {
		c.mu.Lock()
		c.UserName = msg.DisplayName
		c.mu.Unlock()
	}
	if msg.AvatarURL != "" {
		c.mu.Lock()
		c.AvatarURL = msg.AvatarURL
		c.mu.Unlock()
	}
	if msg.Metadata != nil {
		c.mu.Lock()
		c.Metadata = msg.Metadata
		c.mu.Unlock()
	}
	if len(msg.Payload) > 0 {
		var profile struct {
			DisplayName string                 `json:"display_name"`
			UserName    string                 `json:"user_name"`
			AvatarURL   string                 `json:"avatar_url"`
			Role        string                 `json:"role"`
			Metadata    map[string]interface{} `json:"metadata"`
		}
		if err := json.Unmarshal(msg.Payload, &profile); err == nil {
			c.mu.Lock()
			if profile.DisplayName != "" {
				c.UserName = profile.DisplayName
			} else if profile.UserName != "" {
				c.UserName = profile.UserName
			}
			if profile.AvatarURL != "" {
				c.AvatarURL = profile.AvatarURL
			}
			if profile.Role != "" {
				c.Role = profile.Role
			}
			if profile.Metadata != nil {
				c.Metadata = profile.Metadata
			}
			c.mu.Unlock()
		}
	}

	// Extract room_type from message or payload (defaults to "video")
	roomType := msg.RoomType
	if roomType == "" && len(msg.Payload) > 0 {
		var payloadData struct {
			RoomType string `json:"room_type"`
		}
		if err := json.Unmarshal(msg.Payload, &payloadData); err == nil && payloadData.RoomType != "" {
			roomType = payloadData.RoomType
		}
	}
	if roomType != "audio" && roomType != "video" {
		roomType = "video"
	}

	// Retrieve or create room
	room, exists := c.RoomManager.GetRoom(roomID)
	if !exists {
		// If server is in draining mode, reject creating new rooms with 503 Service Unavailable
		if IsServerDraining() {
			log.Printf("[Drain Mode] Rejected new room creation '%s' for host %s: 503 Service Unavailable (Server Draining)\n", roomID, c.ID)
			errPayload, _ := json.Marshal(map[string]any{
				"status_code": 503,
				"error":       "Service Unavailable: Server is draining for shutdown/maintenance",
			})
			if enc, err := (&models.SignalingMessage{
				Event:   "error",
				RoomID:  roomID,
				Payload: errPayload,
			}).Encode(); err == nil {
				c.SafeSend(enc)
			}
			return
		}

		var err error
		room, err = c.RoomManager.CreateRoom(roomID, c.ID)
		if err != nil {
			log.Printf("Failed to create room %s for host %s: %v\n", roomID, c.ID, err)
			return
		}
		if msg.RoomName != "" {
			room.SetRoomName(msg.RoomName)
		}
		room.SetRoomType(roomType)
		log.Printf("Created new room '%s' (Type: %s, Name: %s) for host client %s\n", roomID, roomType, room.GetRoomName(), c.ID)
	} else {
		if msg.RoomName != "" {
			room.SetRoomName(msg.RoomName)
		}
		if msg.RoomType != "" {
			room.SetRoomType(msg.RoomType)
		}
	}

	// Determine if this is Main Host or a Co-Host
	isCoHost := room.HostID != "" && room.HostID != c.ID

	c.mu.Lock()
	existingPC := c.PeerConnection
	c.mu.Unlock()

	if existingPC == nil {
		if vObj, ok := room.GetViewer(c.ID); ok && vObj != nil {
			if vc, ok := vObj.(*Client); ok && vc != nil {
				vc.mu.Lock()
				existingPC = vc.PeerConnection
				vc.mu.Unlock()
				c.mu.Lock()
				c.PeerConnection = existingPC
				c.mu.Unlock()
			}
		}
	}

	var pc *webrtc.PeerConnection
	var err error

	// Seamless Co-Host Upgrade: If user already has an active PeerConnection (e.g. upgraded Viewer),
	// DO NOT tear it down or close it! Reuse and renegotiate on the existing PeerConnection.
	if existingPC != nil && existingPC.ConnectionState() != webrtc.PeerConnectionStateClosed {
		log.Printf("[Seamless Renegotiation] Reusing active PeerConnection for client %s in Room %s\n", c.ID, roomID)
		pc = existingPC
		c.mu.Lock()
		c.Role = "cohost"
		c.mu.Unlock()

		// Configure OnTrack listener to capture newly published co-host streams
		pc.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
			log.Printf("CoHost %s incoming published track [Kind: %s, ID: %s, SSRC: %d, Mime: %s]\n",
				c.ID, remoteTrack.Kind().String(), remoteTrack.ID(), remoteTrack.SSRC(), remoteTrack.Codec().MimeType)

			localTrack, trackErr := webrtc.NewTrackLocalStaticRTP(
				remoteTrack.Codec().RTPCodecCapability,
				remoteTrack.ID(),
				remoteTrack.StreamID(),
			)
			if trackErr != nil {
				log.Printf("Failed to create TrackLocalStaticRTP for cohost %s: %v\n", c.ID, trackErr)
				return
			}

			if remoteTrack.Kind() == webrtc.RTPCodecTypeVideo {
				// Strict Security Check: Drop and ignore video tracks in audio-only rooms
				if room.GetRoomType() == "audio" {
					log.Printf("[Security] Dropped unauthorized co-host %s video track in audio-only Room %s (SSRC: %d)\n",
						c.ID, roomID, remoteTrack.SSRC())
					return
				}
				room.SetCoHostTrack(c.ID, localTrack)
				room.SetCoHostVideoSSRC(c.ID, uint32(remoteTrack.SSRC()))
			} else if remoteTrack.Kind() == webrtc.RTPCodecTypeAudio {
				room.SetCoHostAudioTrack(c.ID, localTrack)
			}

			// Broadcast new_cohost event and trigger server-side track renegotiation
			newCoHostPayload, _ := json.Marshal(map[string]any{
				"event":     "new_cohost",
				"cohost_id": c.ID,
				"room_id":   roomID,
			})
			_ = c.RoomManager.BroadcastToRoom(roomID, &models.SignalingMessage{
				Event:   "new_cohost",
				RoomID:  roomID,
				UserID:  c.ID,
				Payload: newCoHostPayload,
			})

			c.RoomManager.AddTrackAndRenegotiate(roomID, localTrack, c.ID)

			// Forward RTP packets continuously
			go func() {
				bufPtr := internalWebRTC.GetRTPBuffer()
				defer internalWebRTC.PutRTPBuffer(bufPtr)
				buf := *bufPtr
				for {
					n, _, readErr := remoteTrack.Read(buf)
					if readErr != nil {
						return
					}
					if _, writeErr := localTrack.Write(buf[:n]); writeErr != nil {
						return
					}
				}
			}()
		})
	} else if isCoHost {
		log.Printf("Publishing media as fresh Co-Host %s in Room %s\n", c.ID, roomID)
		pc, err = internalWebRTC.HandleCoHostConnection(c.WebRTCAPI, room, c.ID, internalWebRTC.GetDynamicRTCConfiguration(c.ID), func(coHostID string, track *webrtc.TrackLocalStaticRTP) {
			newCoHostPayload, _ := json.Marshal(map[string]any{
				"event":     "new_cohost",
				"cohost_id": coHostID,
				"room_id":   roomID,
			})
			broadcastMsg := &models.SignalingMessage{
				Event:   "new_cohost",
				RoomID:  roomID,
				UserID:  coHostID,
				Payload: newCoHostPayload,
			}
			_ = c.RoomManager.BroadcastToRoom(roomID, broadcastMsg)
			c.RoomManager.AddTrackAndRenegotiate(roomID, track, coHostID)
		})
		if err == nil {
			c.mu.Lock()
			c.PeerConnection = pc
			c.Role = "cohost"
			c.mu.Unlock()
		}
	} else {
		// Main Host
		if room.IsReconnecting() {
			if room.CancelReconnectTimer() {
				log.Printf("Host %s successfully reconnected to Room '%s' within Grace Period!\n", c.ID, roomID)
				reconnectedMsg := &models.SignalingMessage{
					Event:  "host_reconnected",
					RoomID: roomID,
					UserID: c.ID,
				}
				_ = c.RoomManager.BroadcastToRoom(roomID, reconnectedMsg)
			}
		}

		room.SetHostClient(c)
		pc, err = internalWebRTC.HandleHostConnection(c.WebRTCAPI, room, internalWebRTC.GetDynamicRTCConfiguration(c.ID))
		if err == nil {
			c.mu.Lock()
			c.PeerConnection = pc
			c.Role = "host"
			c.mu.Unlock()
		}
	}

	if err != nil {
		log.Printf("Failed to create/setup PeerConnection for client %s: %v\n", c.ID, err)
		return
	}

	// Register ICE and candidate callbacks ONLY if brand new PeerConnection
	if existingPC == nil {
		var restartTimer *time.Timer
		var restartMu sync.Mutex

		pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
			log.Printf("Publisher %s ICE Connection State (Room: %s): %s\n", c.ID, roomID, state.String())
			if state == webrtc.ICEConnectionStateConnected || state == webrtc.ICEConnectionStateCompleted {
				restartMu.Lock()
				if restartTimer != nil {
					restartTimer.Stop()
					restartTimer = nil
				}
				restartMu.Unlock()
				log.Printf("Publisher %s ICE connected/recovered successfully in Room %s\n", c.ID, roomID)
				return
			}
			if state == webrtc.ICEConnectionStateDisconnected || state == webrtc.ICEConnectionStateFailed {
				log.Printf("Publisher %s ICE state '%s' (WiFi-to-Cellular switch or network dip). Scheduling auto ICE restart in 2 seconds...\n", c.ID, state.String())
				restartMu.Lock()
				if restartTimer != nil {
					restartTimer.Stop()
				}
				restartTimer = time.AfterFunc(2*time.Second, func() {
					if pc.ICEConnectionState() == webrtc.ICEConnectionStateDisconnected || pc.ICEConnectionState() == webrtc.ICEConnectionStateFailed {
						log.Printf("[ICE Restart] Auto-triggering ICE restart for publisher %s in Room %s...\n", c.ID, roomID)
						c.TriggerICERestart(roomID)
					}
				})
				restartMu.Unlock()
			}
		})

		pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
			if candidate == nil {
				return
			}
			candJSON, err := json.Marshal(candidate.ToJSON())
			if err != nil {
				log.Printf("Failed to marshal ICE candidate: %v\n", err)
				return
			}

			iceResponse := models.SignalingMessage{
				Event:   models.EventICE,
				RoomID:  roomID,
				UserID:  c.ID,
				Payload: candJSON,
			}
			if encoded, err := iceResponse.Encode(); err == nil {
				c.SafeSend(encoded)
			}
		})
	}

	// Parse SDP offer from payload
	var offer webrtc.SessionDescription
	if err := json.Unmarshal(msg.Payload, &offer); err != nil {
		var sdpWrapper struct {
			SDP  string `json:"sdp"`
			Type string `json:"type"`
		}
		if err2 := json.Unmarshal(msg.Payload, &sdpWrapper); err2 == nil && sdpWrapper.SDP != "" {
			offer = webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  sdpWrapper.SDP,
			}
		} else {
			log.Printf("Failed to parse SDP offer payload from client %s: %v\n", c.ID, err)
			return
		}
	}

	if offer.Type == 0 {
		offer.Type = webrtc.SDPTypeOffer
	}

	// Safety check: verify PeerConnection is valid and not closed before setting remote description
	if pc == nil || pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
		return
	}

	// Set Remote Description (Offer)
	if err := pc.SetRemoteDescription(offer); err != nil {
		if pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
			return
		}
		log.Printf("Failed to set remote description for client %s: %v\n", c.ID, err)
		return
	}

	// Create Answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		log.Printf("Failed to create SDP answer for client %s: %v\n", c.ID, err)
		return
	}

	// Set Local Description (Answer)
	if err := pc.SetLocalDescription(answer); err != nil {
		log.Printf("Failed to set local description for client %s: %v\n", c.ID, err)
		return
	}

	// Send SDP Answer via WebSocket
	answerJSON, err := json.Marshal(answer)
	if err != nil {
		log.Printf("Failed to marshal SDP answer for client %s: %v\n", c.ID, err)
		return
	}

	answerResponse := models.SignalingMessage{
		Event:   models.EventAnswer,
		RoomID:  roomID,
		UserID:  c.ID,
		Payload: answerJSON,
	}

	if encodedResp, err := answerResponse.Encode(); err == nil {
		c.SafeSend(encodedResp)
		log.Printf("Sent SDP answer to client %s for Room %s\n", c.ID, roomID)
	}

	// Trigger immediate keyframe generation
	if room != nil {
		room.SendPLIImmediate()
	}
}

// handleViewerJoinPlay handles a viewer requesting to subscribe/play media from an active room
func (c *Client) handleViewerJoinPlay(msg *models.SignalingMessage) {
	if c.WebRTCAPI == nil {
		log.Printf("WebRTC API not initialized for client %s\n", c.ID)
		return
	}

	roomID := msg.RoomID
	if roomID == "" {
		roomID = "default-room"
	}

	// Access Control: If a user tries to subscribe/play but their token claim says can_subscribe: false, reject.
	if c.Claims != nil && !c.Claims.AllowsSubscribing() {
		log.Printf("[Access Control] Rejected subscribe request from client %s (can_subscribe=false)\n", c.ID)
		errPayload, _ := json.Marshal(map[string]any{
			"status_code": 403,
			"error":       "Forbidden: token claims do not permit subscribing (can_subscribe: false)",
		})
		if enc, err := (&models.SignalingMessage{
			Event:   "error",
			RoomID:  roomID,
			Payload: errPayload,
		}).Encode(); err == nil {
			c.SafeSend(enc)
		}
		return
	}

	// Extract display name, avatar URL, and dynamic metadata if provided
	if msg.DisplayName != "" {
		c.mu.Lock()
		c.UserName = msg.DisplayName
		c.mu.Unlock()
	}
	if msg.AvatarURL != "" {
		c.mu.Lock()
		c.AvatarURL = msg.AvatarURL
		c.mu.Unlock()
	}
	if msg.Metadata != nil {
		c.mu.Lock()
		c.Metadata = msg.Metadata
		c.mu.Unlock()
	}
	if len(msg.Payload) > 0 {
		var profile struct {
			DisplayName string                 `json:"display_name"`
			UserName    string                 `json:"user_name"`
			AvatarURL   string                 `json:"avatar_url"`
			Role        string                 `json:"role"`
			Metadata    map[string]interface{} `json:"metadata"`
		}
		if err := json.Unmarshal(msg.Payload, &profile); err == nil {
			c.mu.Lock()
			if profile.DisplayName != "" {
				c.UserName = profile.DisplayName
			} else if profile.UserName != "" {
				c.UserName = profile.UserName
			}
			if profile.AvatarURL != "" {
				c.AvatarURL = profile.AvatarURL
			}
			if profile.Role != "" {
				c.Role = profile.Role
			}
			if profile.Metadata != nil {
				c.Metadata = profile.Metadata
			}
			c.mu.Unlock()
		}
	}

	// On Edge servers or distributed setups, ensure room is present and cascaded from Origin if needed
	room, exists := c.RoomManager.GetRoom(roomID)
	if !exists {
		var err error
		room, err = c.RoomManager.CreateRoom(roomID, "origin-host")
		if err != nil {
			room, _ = c.RoomManager.GetRoom(roomID)
		}
	}

	if cascadeMgr := c.RoomManager.GetCascadeManager(); cascadeMgr != nil && room != nil {
		_ = cascadeMgr.EnsureCascaded(room)
	}

	// Check if user is banned from this room
	if room.IsUserBanned(c.ID) {
		log.Printf("Viewer join rejected: user %s is banned from room %s\n", c.ID, roomID)
		bannedPayload, _ := json.Marshal(map[string]any{
			"event":   "error",
			"code":    "user_banned",
			"message": "You have been banned from this room by the host",
			"room_id": roomID,
		})
		if enc, err := (&models.SignalingMessage{
			Event:   "error",
			RoomID:  roomID,
			Payload: bannedPayload,
		}).Encode(); err == nil {
			c.SafeSend(enc)
		}
		return
	}

	// Check max viewers limit from config (0 = UNLIMITED)
	appCfg := config.GetAppConfig()
	if appCfg.YAML.RoomManagement.MaxViewersPerRoom > 0 && room.ViewersCount() >= appCfg.YAML.RoomManagement.MaxViewersPerRoom {
		log.Printf("Viewer join rejected: room %s is full (%d/%d viewers)\n", roomID, room.ViewersCount(), appCfg.YAML.RoomManagement.MaxViewersPerRoom)
		errPayload, _ := json.Marshal(map[string]any{
			"event":   "error",
			"code":    "room_full",
			"message": "Room has reached maximum viewer capacity",
			"room_id": roomID,
		})
		if enc, err := (&models.SignalingMessage{
			Event:   "error",
			RoomID:  roomID,
			Payload: errPayload,
		}).Encode(); err == nil {
			c.SafeSend(enc)
		}
		return
	}

	// Check Redis for the room's host node (e.g. Node A vs current Node B)
	var hostNodeID string
	if b := c.RoomManager.GetBroker(); b != nil && b.IsActive() {
		hostNodeID, _ = b.GetRoomNodeMap(context.TODO(), roomID)
	}

	currentNodeID := c.RoomManager.GetNodeID()
	if currentNodeID == "" {
		_, pubAddr := c.RoomManager.GetServerConfig()
		currentNodeID = pubAddr
	}

	// If the host is on Node A and current node is Node B, do NOT initialize a local Pion PeerConnection yet.
	// Instead, serialize Viewer's WebRTC Offer into JSON and publish to Node A via Redis channel signaling.<room_id>.<viewer_id>
	isRemoteHost := hostNodeID != "" && currentNodeID != "" && hostNodeID != currentNodeID
	if isRemoteHost {
		c.mu.Lock()
		c.RoomID = roomID
		c.Role = "viewer"
		c.mu.Unlock()

		log.Printf("[Edge Node Routing] Room %s host is on Node '%s' (current node: '%s'). Serializing Viewer %s WebRTC Offer & publishing to Redis channel 'signaling.%s.%s'\n",
			roomID, hostNodeID, currentNodeID, c.ID, roomID, c.ID)

		// Register viewer in the room signaling state
		_ = c.RoomManager.JoinViewer(roomID, c)

		// Late Joiner Sync: Fetch latest RoomState from Redis and send room_info_sync immediately
		if state := c.RoomManager.GetRoomState(roomID); state != nil {
			stateJSON, _ := json.Marshal(state)
			syncMsg := models.SignalingMessage{
				Event:   "room_info_sync",
				RoomID:  roomID,
				UserID:  c.ID,
				Payload: stateJSON,
			}
			if encoded, err := syncMsg.Encode(); err == nil {
				c.SafeSend(encoded)
			}
		}

		// Serialize Viewer's WebRTC Offer into JSON and publish to Node A via Redis channel
		if b := c.RoomManager.GetBroker(); b != nil && b.IsActive() {
			offerPayload := msg.Payload
			offerMsg := &models.SignalingMessage{
				Event:      "offer",
				RoomID:     roomID,
				UserID:     c.ID,
				Payload:    offerPayload,
				TargetUser: hostNodeID,
			}
			if pubErr := b.PublishViewerSignaling(roomID, c.ID, offerMsg); pubErr != nil {
				log.Printf("[Edge Node] Failed to publish viewer offer to Redis for Room %s: %v\n", roomID, pubErr)
			} else {
				log.Printf("[Edge Node] Published Viewer %s WebRTC Offer to Node A via channel signaling.%s.%s\n", c.ID, roomID, c.ID)
			}

			// Subscribe to return signaling messages (SDP Answer / Host ICE candidates) from Node A
			// Node B relays the Answer and ICE candidates back to the Viewer's WebSocket connection
			_, _ = b.SubscribeViewerSignaling(roomID, c.ID, func(returnMsg *models.SignalingMessage) {
				if returnMsg != nil && returnMsg.UserID != c.ID {
					log.Printf("[Edge Node B] Relaying %s message from Origin Node A back to Viewer %s WebSocket (Room %s)\n",
						returnMsg.Event, c.ID, roomID)
					if encoded, err := returnMsg.Encode(); err == nil {
						c.SafeSend(encoded)
					}
				}
			})
		}

		// On Edge servers, trigger stream cascading from Origin node
		if cascadeMgr := c.RoomManager.GetCascadeManager(); cascadeMgr != nil && room != nil {
			_ = cascadeMgr.EnsureCascaded(room)
		}
		return
	}

	// Setup PeerConnection with host's tracks attached and dynamic TURN REST credentials
	pc, err := internalWebRTC.HandleViewerConnection(c.WebRTCAPI, c.RoomManager, roomID, internalWebRTC.GetDynamicRTCConfiguration(c.ID))
	if err != nil {
		log.Printf("Failed to setup viewer PeerConnection for client %s (Room %s): %v\n", c.ID, roomID, err)
		return
	}

	// Register viewer in the room
	_ = c.RoomManager.JoinViewer(roomID, c)

	// Late Joiner Sync: Fetch latest RoomState from Redis and send room_info_sync immediately
	if state := c.RoomManager.GetRoomState(roomID); state != nil {
		stateJSON, _ := json.Marshal(state)
		syncMsg := models.SignalingMessage{
			Event:   "room_info_sync",
			RoomID:  roomID,
			UserID:  c.ID,
			Payload: stateJSON,
		}
		if encoded, err := syncMsg.Encode(); err == nil {
			c.SafeSend(encoded)
		}
	}

	c.mu.Lock()
	c.PeerConnection = pc
	c.mu.Unlock()

	// Graceful ICE Connection State handling with auto ICE restart
	var restartTimer *time.Timer
	var restartMu sync.Mutex

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("Viewer %s ICE Connection State (Room: %s): %s\n", c.ID, roomID, state.String())
		if state == webrtc.ICEConnectionStateConnected || state == webrtc.ICEConnectionStateCompleted {
			restartMu.Lock()
			if restartTimer != nil {
				restartTimer.Stop()
				restartTimer = nil
			}
			restartMu.Unlock()
			log.Printf("Viewer %s ICE connected/recovered successfully in Room %s\n", c.ID, roomID)
			return
		}
		if state == webrtc.ICEConnectionStateDisconnected || state == webrtc.ICEConnectionStateFailed {
			log.Printf("Viewer %s ICE state '%s' (WiFi-to-Cellular switch or network dip). Scheduling auto ICE restart in 2 seconds...\n", c.ID, state.String())
			restartMu.Lock()
			if restartTimer != nil {
				restartTimer.Stop()
			}
			restartTimer = time.AfterFunc(2*time.Second, func() {
				if pc.ICEConnectionState() == webrtc.ICEConnectionStateDisconnected || pc.ICEConnectionState() == webrtc.ICEConnectionStateFailed {
					log.Printf("[ICE Restart] Auto-triggering ICE restart for viewer %s in Room %s...\n", c.ID, roomID)
					c.TriggerICERestart(roomID)
				}
			})
			restartMu.Unlock()
		}
	})

	// Send server-side ICE candidates to the viewer via WebSocket
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		candJSON, err := json.Marshal(candidate.ToJSON())
		if err != nil {
			log.Printf("Failed to marshal viewer ICE candidate: %v\n", err)
			return
		}

		iceResponse := models.SignalingMessage{
			Event:   models.EventICE,
			RoomID:  roomID,
			UserID:  c.ID,
			Payload: candJSON,
		}
		if encoded, err := iceResponse.Encode(); err == nil {
			c.SafeSend(encoded)
		}
	})

	// Parse SDP offer from viewer payload
	var offer webrtc.SessionDescription
	if err := json.Unmarshal(msg.Payload, &offer); err != nil {
		var sdpWrapper struct {
			SDP  string `json:"sdp"`
			Type string `json:"type"`
		}
		if err2 := json.Unmarshal(msg.Payload, &sdpWrapper); err2 == nil && sdpWrapper.SDP != "" {
			offer = webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  sdpWrapper.SDP,
			}
		} else {
			log.Printf("Failed to parse viewer SDP offer from client %s: %v\n", c.ID, err)
			return
		}
	}

	if offer.Type == 0 {
		offer.Type = webrtc.SDPTypeOffer
	}

	// Safety check: verify PeerConnection is valid and not closed before setting remote description
	if pc == nil || pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
		return
	}

	// Set Remote Description (Viewer's Offer)
	if err := pc.SetRemoteDescription(offer); err != nil {
		if pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
			return
		}
		log.Printf("Failed to set remote description for viewer %s: %v\n", c.ID, err)
		return
	}

	// Generate SDP Answer for the viewer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		log.Printf("Failed to create SDP answer for viewer %s: %v\n", c.ID, err)
		return
	}

	// Set Local Description (Answer)
	if err := pc.SetLocalDescription(answer); err != nil {
		log.Printf("Failed to set local description for viewer %s: %v\n", c.ID, err)
		return
	}

	// Send SDP Answer to Viewer via WebSocket
	answerJSON, err := json.Marshal(answer)
	if err != nil {
		log.Printf("Failed to marshal SDP answer for viewer %s: %v\n", c.ID, err)
		return
	}

	answerResponse := models.SignalingMessage{
		Event:   models.EventAnswer,
		RoomID:  roomID,
		UserID:  c.ID,
		Payload: answerJSON,
	}

	if encodedResp, err := answerResponse.Encode(); err == nil {
		c.SafeSend(encodedResp)
		log.Printf("Sent SDP answer to viewer client %s for Room %s\n", c.ID, roomID)
	}

	// Trigger immediate keyframe generation for crystal clear HD video on viewer join
	if room != nil {
		room.SendPLIImmediate()
	}

	// Broadcast updated viewer count to everyone in the room (Host and all Viewers)
	roomObj, roomFound := c.RoomManager.GetRoom(roomID)
	if roomFound && roomObj != nil {
		viewerCountPayload, _ := json.Marshal(map[string]any{
			"event":   "viewer_count",
			"room_id": roomID,
			"count":   roomObj.ViewersCount(),
		})
		_ = c.RoomManager.BroadcastToRoom(roomID, &models.SignalingMessage{
			Event:   "viewer_count",
			RoomID:  roomID,
			Payload: viewerCountPayload,
		})

		// Send room_info sync message to late-joining viewer
		roomInfoPayload, _ := json.Marshal(map[string]any{
			"event":          "room_info",
			"room_id":        roomID,
			"created_at":     roomObj.CreatedAt.Format(time.RFC3339),
			"host_id":        roomObj.HostID,
			"main_seat_id":   roomObj.GetMainSeatID(),
			"host_score":     roomObj.GetHostScore(),
			"active_cohosts": roomObj.GetActiveCoHostIDs(),
			"viewer_count":   roomObj.ViewersCount(),
		})
		roomInfoMsg := models.SignalingMessage{
			Event:   "room_info",
			RoomID:  roomID,
			UserID:  c.ID,
			Payload: roomInfoPayload,
		}
		if encoded, err := roomInfoMsg.Encode(); err == nil {
			c.Send <- encoded
		}
		log.Printf("Sent room_info sync to late joiner viewer %s in Room %s\n", c.ID, roomID)
	}
}

// handleSetMainSeat changes the active main seat participant and broadcasts to the room
func (c *Client) handleSetMainSeat(msg *models.SignalingMessage) {
	roomID := msg.RoomID
	if roomID == "" {
		log.Printf("set_main_seat rejected: missing room_id\n")
		return
	}

	room, exists := c.RoomManager.GetRoom(roomID)
	if !exists || room == nil {
		log.Printf("set_main_seat rejected: room %s not found\n", roomID)
		return
	}

	targetID := msg.TargetUser
	if targetID == "" && len(msg.Payload) > 0 {
		var payloadData struct {
			TargetID   string `json:"target_id"`
			TargetUser string `json:"target_user"`
			CoHostID   string `json:"cohost_id"`
		}
		if err := json.Unmarshal(msg.Payload, &payloadData); err == nil {
			if payloadData.TargetID != "" {
				targetID = payloadData.TargetID
			} else if payloadData.TargetUser != "" {
				targetID = payloadData.TargetUser
			} else if payloadData.CoHostID != "" {
				targetID = payloadData.CoHostID
			}
		}
	}

	if targetID == "" {
		log.Printf("set_main_seat rejected: missing target_id in message\n")
		return
	}

	room.SetMainSeatID(targetID)
	log.Printf("Main seat for Room %s updated to: %s by %s\n", roomID, targetID, c.ID)

	broadcastPayload, _ := json.Marshal(map[string]any{
		"event":        "main_seat_changed",
		"room_id":      roomID,
		"target_id":    targetID,
		"main_seat_id": targetID,
	})

	_ = c.RoomManager.BroadcastToRoom(roomID, &models.SignalingMessage{
		Event:      "main_seat_changed",
		RoomID:     roomID,
		UserID:     c.ID,
		TargetUser: targetID,
		Payload:    broadcastPayload,
	})
}

// handleChatMessage broadcasts live room chat messages to host and all viewers (and cross-room DataChannels if linked in PK)
func (c *Client) handleChatMessage(msg *models.SignalingMessage) {
	roomID := msg.RoomID
	if roomID == "" {
		roomID = c.RoomID
	}
	if roomID == "" {
		log.Printf("Chat message rejected: missing room_id from client %s\n", c.ID)
		return
	}

	// Attach sender user id if not provided
	if msg.UserID == "" {
		msg.UserID = c.ID
	}

	// Broadcast chat message to host and all viewers in the room (and linked PK room)
	c.RoomManager.BroadcastSignalingToLinkedRooms(roomID, msg)

	// Cross-broadcast raw chat message over WebRTC DataChannel ("room-events") to linked rooms
	if encoded, err := msg.Encode(); err == nil {
		c.RoomManager.BroadcastToLinkedRooms(roomID, c.ID, "room-events", encoded)
	}

	// Store bounded chat in Redis (strictly kept at max 50 items with LTRIM and 24h TTL)
	if broker := c.RoomManager.GetBroker(); broker != nil && broker.IsActive() {
		_ = broker.PushChatMessage(context.TODO(), roomID, msg)
	}

	log.Printf("Broadcasted chat message from user %s in Room %s (linked mesh)\n", c.ID, roomID)
}

// handleGiftMessage processes targeted gift donations, increments target host score, and broadcasts pk_gift_overlay to both rooms
func (c *Client) handleGiftMessage(msg *models.SignalingMessage) {
	roomID := msg.RoomID
	if roomID == "" {
		roomID = c.RoomID
	}
	if roomID == "" {
		log.Printf("Gift message rejected: missing room_id from client %s\n", c.ID)
		return
	}

	room, exists := c.RoomManager.GetRoom(roomID)
	if !exists || room == nil {
		log.Printf("Gift message rejected: room %s not found\n", roomID)
		return
	}

	// Parse gift coins/points/value and target_host_id from payload
	var giftPayload struct {
		GiftID       string `json:"gift_id"`
		Gift         string `json:"gift"`
		TargetHostID string `json:"target_host_id"`
		ReceiverID   string `json:"receiver_id"`
		Coins        int    `json:"coins"`
		Points       int    `json:"points"`
		Amount       int    `json:"amount"`
		Value        int    `json:"value"`
	}

	points := 10 // default points if not explicitly specified
	giftName := "gift"
	targetHostID := room.HostID

	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &giftPayload); err == nil {
			if giftPayload.Coins > 0 {
				points = giftPayload.Coins
			} else if giftPayload.Points > 0 {
				points = giftPayload.Points
			} else if giftPayload.Value > 0 {
				points = giftPayload.Value
			} else if giftPayload.Amount > 0 {
				points = giftPayload.Amount
			}

			if giftPayload.GiftID != "" {
				giftName = giftPayload.GiftID
			} else if giftPayload.Gift != "" {
				giftName = giftPayload.Gift
			}

			if giftPayload.TargetHostID != "" {
				targetHostID = giftPayload.TargetHostID
			} else if giftPayload.ReceiverID != "" {
				targetHostID = giftPayload.ReceiverID
			}
		}
	}

	// Determine recipient room for targeted gifting
	targetRoomID := roomID
	if targetHostID != "" && targetHostID != room.HostID {
		// Check linked room or find target room by host
		linkedID := room.GetLinkedRoom()
		if linkedID != "" {
			if linkedRoom, ok := c.RoomManager.GetRoom(linkedID); ok && linkedRoom != nil {
				if linkedRoom.HostID == targetHostID {
					targetRoomID = linkedID
				}
			}
		}
		if targetRoomID == roomID && c.RoomManager.pkManager != nil {
			if session, ok := c.RoomManager.pkManager.GetPKSession(roomID); ok && session != nil {
				if session.HostID1 == targetHostID {
					targetRoomID = session.RoomID1
				} else if session.HostID2 == targetHostID {
					targetRoomID = session.RoomID2
				}
			}
		}
	}

	// Increment target host score atomically in Redis and update room state
	newScore := c.RoomManager.AddGiftScore(targetRoomID, int64(points))

	senderID := msg.UserID
	if senderID == "" {
		senderID = c.ID
	}
	senderName := c.UserName
	if senderName == "" {
		senderName = senderID
	}

	// Check if this room is in an active PK battle
	var pkSession *models.PKSession
	if c.RoomManager.pkManager != nil {
		pkSession, _ = c.RoomManager.pkManager.GetPKSession(roomID)
	}

	if pkSession != nil {
		// Sync PK battle scores for both rooms
		hostAPoints := int64(0)
		hostBPoints := int64(0)

		if r1, ok := c.RoomManager.GetRoom(pkSession.RoomID1); ok && r1 != nil {
			hostAPoints = int64(r1.GetHostScore())
		}
		if r2, ok := c.RoomManager.GetRoom(pkSession.RoomID2); ok && r2 != nil {
			hostBPoints = int64(r2.GetHostScore())
		}

		pkOverlayPayload, _ := json.Marshal(map[string]any{
			"type":           "pk_gift_overlay",
			"event":          "pk_gift_overlay",
			"session_id":     pkSession.SessionID,
			"sender_name":    senderName,
			"sender_id":      senderID,
			"gift":           giftName,
			"gift_id":        giftName,
			"receiver_id":    targetHostID,
			"target_host_id": targetHostID,
			"host_a_points":  hostAPoints,
			"host_b_points":  hostBPoints,
			"room_a_score":   hostAPoints,
			"room_b_score":   hostBPoints,
			"coins":          points,
			"points_added":   points,
		})

		overlayMsg := &models.SignalingMessage{
			Action:  "pk_gift_overlay",
			Event:   "pk_gift_overlay",
			RoomID:  roomID,
			UserID:  senderID,
			Payload: pkOverlayPayload,
		}

		// 1. Broadcast PK overlay to BOTH rooms simultaneously via WebSocket
		c.RoomManager.BroadcastSignalingToLinkedRooms(roomID, overlayMsg)

		// 2. Broadcast PK overlay to BOTH rooms simultaneously via DataChannels
		c.RoomManager.BroadcastToLinkedRooms(roomID, senderID, "room-events", pkOverlayPayload)

		// 3. Update real-time score sync
		c.RoomManager.pkManager.SyncPKScore(roomID, hostAPoints)

	} else {
		// Standard single-room gift processing
		respPayload, err := json.Marshal(map[string]any{
			"event":        "gift_processed",
			"sender":       senderID,
			"sender_id":    senderID,
			"sender_name":  senderName,
			"gift_id":      giftName,
			"coins":        points,
			"points_added": points,
			"new_score":    newScore,
			"room_id":      targetRoomID,
			"host_id":      targetHostID,
		})
		if err != nil {
			log.Printf("Failed to marshal gift_processed payload: %v\n", err)
			return
		}

		broadcastMsg := &models.SignalingMessage{
			Event:   "gift_processed",
			RoomID:  targetRoomID,
			UserID:  senderID,
			Payload: respPayload,
		}

		_ = c.RoomManager.BroadcastToRoom(targetRoomID, broadcastMsg)
		c.RoomManager.BroadcastToLinkedRooms(targetRoomID, senderID, "room-events", respPayload)
	}

	// Trigger GiftSent webhook event
	if webhookDispatcher := c.RoomManager.GetWebhookDispatcher(); webhookDispatcher != nil {
		webhookDispatcher.Dispatch(api.WebhookEvent{
			EventType: api.EventGiftSent,
			RoomID:    targetRoomID,
			UserID:    senderID,
			Data: map[string]any{
				"gift_id":        giftName,
				"coins":          points,
				"host_id":        targetHostID,
				"target_host_id": targetHostID,
				"new_score":      newScore,
			},
		})
	}

	log.Printf("Gift '%s' processed for Host %s (Room: %s) from %s (added: %d, new_score: %d)\n", giftName, targetHostID, targetRoomID, senderID, points, newScore)
}

// HandleGiftMessageDirect public entrypoint for processing gifts from REST API or tests
func (c *Client) HandleGiftMessageDirect(msg *models.SignalingMessage) {
	c.handleGiftMessage(msg)
}

// handleRoomStateSync sends the current cached RoomState to the requesting client
func (c *Client) handleRoomStateSync(msg *models.SignalingMessage) {
	roomID := msg.RoomID
	if roomID == "" {
		return
	}
	state := c.RoomManager.GetRoomState(roomID)
	if state == nil {
		return
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return
	}
	syncMsg := models.SignalingMessage{
		Event:   "room_info_sync",
		RoomID:  roomID,
		UserID:  c.ID,
		Payload: stateJSON,
	}
	if encoded, err := syncMsg.Encode(); err == nil {
		c.Send <- encoded
	}
}

// handleMediaStateChange updates a client's mute/unmute state and broadcasts to the room via WebSocket & DataChannel
func (c *Client) handleMediaStateChange(msg *models.SignalingMessage) {
	roomID := msg.RoomID
	if roomID == "" {
		roomID = c.RoomID
	}
	if roomID == "" {
		return
	}

	var parsedReq struct {
		Type       string `json:"type"`
		Event      string `json:"event"`
		TrackID    string `json:"track_id"`
		Muted      *bool  `json:"muted"`
		Kind       string `json:"kind"` // "video" or "audio"
		MutedAudio *bool  `json:"muted_audio"`
		MutedVideo *bool  `json:"muted_video"`
	}

	if len(msg.Payload) > 0 {
		_ = json.Unmarshal(msg.Payload, &parsedReq)
	}

	currentState := models.MediaState{}
	if room, exists := c.RoomManager.GetRoom(roomID); exists && room != nil {
		if st, found := room.GetMediaState(c.ID); found {
			currentState = st
		}
	}

	isMuted := false
	if parsedReq.Muted != nil {
		isMuted = *parsedReq.Muted
	} else if msg.Event == "track_muted" {
		isMuted = true
	} else if msg.Event == "track_unmuted" {
		isMuted = false
	}

	trackKind := parsedReq.Kind
	if trackKind == "" {
		if parsedReq.MutedAudio != nil {
			trackKind = "audio"
			isMuted = *parsedReq.MutedAudio
		} else if parsedReq.MutedVideo != nil {
			trackKind = "video"
			isMuted = *parsedReq.MutedVideo
		} else {
			trackKind = "video"
		}
	}

	if trackKind == "audio" {
		currentState.MutedAudio = isMuted
	} else {
		currentState.MutedVideo = isMuted
	}

	c.RoomManager.SetMediaState(roomID, c.ID, currentState)

	roomState := c.RoomManager.GetRoomState(roomID)
	var mediaStates map[string]models.MediaState
	if roomState != nil {
		mediaStates = roomState.MediaStates
	}

	trackID := parsedReq.TrackID
	if trackID == "" {
		trackID = fmt.Sprintf("%s_%s", c.ID, trackKind)
	}

	payload, _ := json.Marshal(map[string]any{
		"type":         "track_muted",
		"event":        "track_muted",
		"track_id":     trackID,
		"muted":        isMuted,
		"kind":         trackKind,
		"user_id":      c.ID,
		"muted_audio":  currentState.MutedAudio,
		"muted_video":  currentState.MutedVideo,
		"media_states": mediaStates,
	})

	// 1. Broadcast to all Viewers via WebSocket
	_ = c.RoomManager.BroadcastToRoom(roomID, &models.SignalingMessage{
		Action:  "track_muted",
		Event:   "track_muted",
		RoomID:  roomID,
		UserID:  c.ID,
		Payload: payload,
	})

	// 2. Broadcast immediately to all Viewers via WebRTC DataChannel ("room-events")
	if room, exists := c.RoomManager.GetRoom(roomID); exists && room != nil {
		room.BroadcastDataChannelMessage(c.ID, "room-events", payload)
	}
}

// handleSeatRequest forwards a viewer's co-host / seat request directly to the room's host
func (c *Client) handleSeatRequest(msg *models.SignalingMessage) {
	roomID := msg.RoomID
	if roomID == "" {
		log.Printf("Seat request rejected: missing room_id from client %s\n", c.ID)
		return
	}

	room, exists := c.RoomManager.GetRoom(roomID)
	if !exists || room == nil {
		log.Printf("Seat request rejected: room %s not found\n", roomID)
		return
	}

	senderID := msg.UserID
	if senderID == "" {
		senderID = c.ID
		msg.UserID = senderID
	}

	// Backend Seat Request Guard: Check if host is nil, disconnected or in Grace Period reconnecting
	if room.HostClient == nil || room.IsReconnecting() {
		log.Printf("Seat request rejected: Host is offline or reconnecting in Room %s\n", roomID)
		errPayload, _ := json.Marshal(map[string]any{
			"error":   "Host is currently offline",
			"message": "Host is currently offline. Please wait until host reconnects.",
		})
		errMsg := models.SignalingMessage{
			Event:   "error",
			RoomID:  roomID,
			UserID:  senderID,
			Payload: errPayload,
		}
		if encoded, err := errMsg.Encode(); err == nil {
			select {
			case c.Send <- encoded:
			default:
				log.Printf("Viewer send buffer full while returning seat_request error\n")
			}
		}
		return
	}

	// Forward seat_request to Host
	if hostClient, ok := room.HostClient.(*Client); ok && hostClient != nil {
		encoded, err := msg.Encode()
		if err == nil {
			select {
			case hostClient.Send <- encoded:
				log.Printf("Relayed seat_request from %s to Host %s in Room %s\n", senderID, room.HostID, roomID)
			default:
				log.Printf("Host send buffer full during seat_request\n")
			}
			return
		}
	}

	log.Printf("Host client not available in Room %s for seat_request\n", roomID)
	errPayload, _ := json.Marshal(map[string]any{
		"error":   "Host is currently offline",
		"message": "Host is currently offline",
	})
	errMsg := models.SignalingMessage{
		Event:   "error",
		RoomID:  roomID,
		UserID:  senderID,
		Payload: errPayload,
	}
	if encoded, err := errMsg.Encode(); err == nil {
		c.Send <- encoded
	}
}

// handleSeatAccept upgrades a viewer to co-host, assigns an active seat, and notifies participants
func (c *Client) handleSeatAccept(msg *models.SignalingMessage) {
	roomID := msg.RoomID
	if roomID == "" {
		log.Printf("Seat accept rejected: missing room_id from client %s\n", c.ID)
		return
	}

	room, exists := c.RoomManager.GetRoom(roomID)
	if !exists || room == nil {
		log.Printf("Seat accept rejected: room %s not found\n", roomID)
		return
	}

	// Verify that only the room host can accept seat requests
	if c.ID != room.HostID {
		log.Printf("Seat accept rejected: client %s is not host of room %s\n", c.ID, roomID)
		return
	}

	targetUser := msg.TargetUser
	if targetUser == "" && len(msg.Payload) > 0 {
		var payloadData struct {
			TargetUser string `json:"target_user"`
			UserID     string `json:"user_id"`
		}
		if err := json.Unmarshal(msg.Payload, &payloadData); err == nil {
			if payloadData.TargetUser != "" {
				targetUser = payloadData.TargetUser
			} else if payloadData.UserID != "" {
				targetUser = payloadData.UserID
			}
		}
	}

	if targetUser == "" {
		log.Printf("Seat accept rejected: missing target_user in message\n")
		return
	}

	// Find available seat based on CoHosting config (0 = UNLIMITED)
	appCfg := config.GetAppConfig()
	maxSeats := appCfg.YAML.CoHosting.MaxActiveSeats
	activeSeats := room.GetActiveSeats()
	assignedSeat := ""

	if maxSeats > 0 {
		for i := 1; i <= maxSeats; i++ {
			seatKey := fmt.Sprintf("%d", i)
			if _, taken := activeSeats[seatKey]; !taken {
				assignedSeat = seatKey
				break
			}
		}
	} else {
		// UNLIMITED co-host seats (0): search for next available positive integer key
		for i := 1; ; i++ {
			seatKey := fmt.Sprintf("%d", i)
			if _, taken := activeSeats[seatKey]; !taken {
				assignedSeat = seatKey
				break
			}
		}
	}

	if assignedSeat == "" {
		log.Printf("Seat accept rejected: all %d seats are occupied in room %s\n", maxSeats, roomID)
		return
	}

	// Assign seat to target user
	room.SetActiveSeat(assignedSeat, targetUser)
	c.RoomManager.SyncRoomState(roomID)

	// Upgrade role if viewer is connected locally
	var targetClient *Client
	viewerObj, found := room.GetViewer(targetUser)
	if found && viewerObj != nil {
		if vc, ok := viewerObj.(*Client); ok && vc != nil {
			targetClient = vc
			targetClient.mu.Lock()
			targetClient.Role = "cohost"
			targetClient.mu.Unlock()
		}
	}

	// Send seat_accept directly to the target viewer
	acceptPayload, _ := json.Marshal(map[string]any{
		"event":       "seat_accept",
		"room_id":     roomID,
		"target_user": targetUser,
		"user_id":     targetUser,
		"seat_id":     assignedSeat,
	})

	acceptMsg := models.SignalingMessage{
		Event:      "seat_accept",
		RoomID:     roomID,
		TargetUser: targetUser,
		Payload:    acceptPayload,
	}

	if targetClient != nil {
		if enc, err := acceptMsg.Encode(); err == nil {
			select {
			case targetClient.Send <- enc:
				log.Printf("Sent seat_accept (seat %s) to %s in Room %s\n", assignedSeat, targetUser, roomID)
			default:
			}
		}
	}

	// Broadcast seat_updated to all room members
	updatePayload, _ := json.Marshal(map[string]any{
		"event":        "seat_updated",
		"action":       "join",
		"user_id":      targetUser,
		"seat_id":      assignedSeat,
		"active_seats": room.GetActiveSeats(),
	})
	_ = c.RoomManager.BroadcastToRoom(roomID, &models.SignalingMessage{
		Event:   "seat_updated",
		RoomID:  roomID,
		UserID:  targetUser,
		Payload: updatePayload,
	})
}

// handleLeaveSeat handles a Co-Host stepping down voluntarily
func (c *Client) handleLeaveSeat(msg *models.SignalingMessage) {
	roomID := msg.RoomID
	if roomID == "" {
		return
	}

	room, exists := c.RoomManager.GetRoom(roomID)
	if !exists || room == nil {
		return
	}

	// Downgrade client role to viewer
	c.mu.Lock()
	c.Role = "viewer"
	c.mu.Unlock()

	// Remove co-host tracks and seats from SFU & Redis
	c.RoomManager.RemoveTrackAndRenegotiate(roomID, c.ID)

	// Send confirmation to client
	leavePayload, _ := json.Marshal(map[string]any{
		"event":   "seat_left",
		"room_id": roomID,
		"user_id": c.ID,
	})
	if enc, err := (&models.SignalingMessage{
		Event:   "seat_left",
		RoomID:  roomID,
		UserID:  c.ID,
		Payload: leavePayload,
	}).Encode(); err == nil {
		select {
		case c.Send <- enc:
		default:
		}
	}

	// Broadcast seat_updated and cohost_left to all room members
	updatePayload, _ := json.Marshal(map[string]any{
		"event":        "seat_updated",
		"action":       "leave",
		"user_id":      c.ID,
		"cohost_id":    c.ID,
		"active_seats": room.GetActiveSeats(),
	})
	_ = c.RoomManager.BroadcastToRoom(roomID, &models.SignalingMessage{
		Event:   "seat_updated",
		RoomID:  roomID,
		UserID:  c.ID,
		Payload: updatePayload,
	})
	_ = c.RoomManager.BroadcastToRoom(roomID, &models.SignalingMessage{
		Event:   "cohost_left",
		RoomID:  roomID,
		UserID:  c.ID,
		Payload: updatePayload,
	})
	log.Printf("Co-Host %s stepped down from seat in Room %s\n", c.ID, roomID)
}

// handleKickSeat handles the Host kicking a Co-Host from their seat
func (c *Client) handleKickSeat(msg *models.SignalingMessage) {
	roomID := msg.RoomID
	if roomID == "" {
		return
	}

	room, exists := c.RoomManager.GetRoom(roomID)
	if !exists || room == nil {
		return
	}

	// Only host can kick co-hosts
	if c.ID != room.HostID {
		log.Printf("Kick rejected: client %s is not host of room %s\n", c.ID, roomID)
		return
	}

	targetUser := msg.TargetUser
	if targetUser == "" && len(msg.Payload) > 0 {
		var payloadData struct {
			TargetUser string `json:"target_user"`
			UserID     string `json:"user_id"`
		}
		if err := json.Unmarshal(msg.Payload, &payloadData); err == nil {
			if payloadData.TargetUser != "" {
				targetUser = payloadData.TargetUser
			} else if payloadData.UserID != "" {
				targetUser = payloadData.UserID
			}
		}
	}

	if targetUser == "" {
		return
	}

	// Remove co-host tracks and seat from SFU & Redis
	c.RoomManager.RemoveTrackAndRenegotiate(roomID, targetUser)

	// Downgrade and notify kicked user
	if viewerObj, found := room.GetViewer(targetUser); found && viewerObj != nil {
		if vc, ok := viewerObj.(*Client); ok && vc != nil {
			vc.mu.Lock()
			vc.Role = "viewer"
			vc.mu.Unlock()

			kickedPayload, _ := json.Marshal(map[string]any{
				"event":   "seat_kicked",
				"room_id": roomID,
				"user_id": targetUser,
			})
			if enc, err := (&models.SignalingMessage{
				Event:   "seat_kicked",
				RoomID:  roomID,
				UserID:  targetUser,
				Payload: kickedPayload,
			}).Encode(); err == nil {
				select {
				case vc.Send <- enc:
				default:
				}
			}
		}
	}

	// Broadcast seat_updated and cohost_left to all room members
	updatePayload, _ := json.Marshal(map[string]any{
		"event":        "seat_updated",
		"action":       "kick",
		"user_id":      targetUser,
		"cohost_id":    targetUser,
		"active_seats": room.GetActiveSeats(),
	})
	_ = c.RoomManager.BroadcastToRoom(roomID, &models.SignalingMessage{
		Event:   "seat_updated",
		RoomID:  roomID,
		UserID:  targetUser,
		Payload: updatePayload,
	})
	_ = c.RoomManager.BroadcastToRoom(roomID, &models.SignalingMessage{
		Event:   "cohost_left",
		RoomID:  roomID,
		UserID:  targetUser,
		Payload: updatePayload,
	})
	log.Printf("Host %s kicked Co-Host %s from seat in Room %s\n", c.ID, targetUser, roomID)
}

// handleSubscribeCoHost attaches a specific co-host track to the requesting client's PeerConnection
func (c *Client) handleSubscribeCoHost(msg *models.SignalingMessage) {
	roomID := msg.RoomID
	if roomID == "" {
		log.Printf("subscribe_cohost rejected: missing room_id\n")
		return
	}

	room, exists := c.RoomManager.GetRoom(roomID)
	if !exists || room == nil {
		log.Printf("subscribe_cohost rejected: room %s not found\n", roomID)
		return
	}

	coHostID := msg.TargetUser
	if coHostID == "" && len(msg.Payload) > 0 {
		var payloadData struct {
			CoHostID   string `json:"cohost_id"`
			TargetUser string `json:"target_user"`
		}
		if err := json.Unmarshal(msg.Payload, &payloadData); err == nil {
			if payloadData.CoHostID != "" {
				coHostID = payloadData.CoHostID
			} else if payloadData.TargetUser != "" {
				coHostID = payloadData.TargetUser
			}
		}
	}

	if coHostID == "" {
		log.Printf("subscribe_cohost rejected: missing cohost_id\n")
		return
	}

	coHostTrack, trackFound := room.GetCoHostTrack(coHostID)
	if !trackFound || coHostTrack == nil {
		log.Printf("subscribe_cohost rejected: track for cohost %s not found in room %s\n", coHostID, roomID)
		return
	}

	c.mu.Lock()
	pc := c.PeerConnection
	c.mu.Unlock()

	if pc == nil {
		var err error
		pc, err = internalWebRTC.HandleViewerConnection(c.WebRTCAPI, c.RoomManager, roomID, internalWebRTC.GetDynamicRTCConfiguration(c.ID))
		if err != nil {
			log.Printf("Failed to create PeerConnection for subscribe_cohost: %v\n", err)
			return
		}
		c.mu.Lock()
		c.PeerConnection = pc
		c.mu.Unlock()
	}

	// Add co-host track to client's PeerConnection
	sender, err := pc.AddTrack(coHostTrack)
	if err != nil {
		log.Printf("Failed to add co-host track %s to client %s: %v\n", coHostID, c.ID, err)
		return
	}

	go func() {
		bufPtr := internalWebRTC.GetRTPBuffer()
		defer internalWebRTC.PutRTPBuffer(bufPtr)
		buf := *bufPtr

		for {
			if _, _, rtcpErr := sender.Read(buf); rtcpErr != nil {
				return
			}
		}
	}()

	// Parse SDP offer if provided
	var offer webrtc.SessionDescription
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &offer); err != nil {
			var sdpWrapper struct {
				SDP  string `json:"sdp"`
				Type string `json:"type"`
			}
			if err2 := json.Unmarshal(msg.Payload, &sdpWrapper); err2 == nil && sdpWrapper.SDP != "" {
				offer = webrtc.SessionDescription{
					Type: webrtc.SDPTypeOffer,
					SDP:  sdpWrapper.SDP,
				}
			}
		}
	}

	if offer.SDP != "" {
		if offer.Type == 0 {
			offer.Type = webrtc.SDPTypeOffer
		}

		// Safety check: verify PeerConnection is valid and open
		if pc == nil || pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
			return
		}

		if err := pc.SetRemoteDescription(offer); err != nil {
			if pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
				return
			}
			log.Printf("Failed to set remote description for subscribe_cohost: %v\n", err)
			return
		}

		answer, err := pc.CreateAnswer(nil)
		if err != nil {
			if pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
				return
			}
			log.Printf("Failed to create SDP answer for subscribe_cohost: %v\n", err)
			return
		}

		if err := pc.SetLocalDescription(answer); err != nil {
			if pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
				return
			}
			log.Printf("Failed to set local description for subscribe_cohost: %v\n", err)
			return
		}

		answerJSON, err := json.Marshal(answer)
		if err != nil {
			return
		}

		resp := models.SignalingMessage{
			Event:      models.EventAnswer,
			RoomID:     roomID,
			UserID:     c.ID,
			TargetUser: coHostID,
			Payload:    answerJSON,
		}
		if encoded, err := resp.Encode(); err == nil {
			c.Send <- encoded
		}
		log.Printf("Successfully subscribed client %s to CoHost %s (answer sent)\n", c.ID, coHostID)
	}
}

// handleICECandidate receives and adds remote ICE candidate to the PeerConnection
func (c *Client) handleICECandidate(msg *models.SignalingMessage) {
	c.mu.Lock()
	pc := c.PeerConnection
	c.mu.Unlock()

	// If PeerConnection is nil on this node (e.g. Edge Node B), check if Room host is on Node A and publish ICE candidate via Redis channel
	if pc == nil || pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
		if b := c.RoomManager.GetBroker(); b != nil && b.IsActive() {
			roomID := msg.RoomID
			if roomID == "" {
				c.mu.Lock()
				roomID = c.RoomID
				c.mu.Unlock()
			}
			if roomID != "" {
				hostNodeID, _ := b.GetRoomNodeMap(context.TODO(), roomID)
				currentNodeID := c.RoomManager.GetNodeID()
				if currentNodeID == "" {
					_, pubAddr := c.RoomManager.GetServerConfig()
					currentNodeID = pubAddr
				}
				if hostNodeID != "" && currentNodeID != "" && hostNodeID != currentNodeID {
					// Serialize Viewer's ICE candidate into JSON and publish to Node A via Redis channel
					iceMsg := &models.SignalingMessage{
						Event:      "ice",
						RoomID:     roomID,
						UserID:     c.ID,
						Payload:    msg.Payload,
						TargetUser: hostNodeID,
					}
					if err := b.PublishViewerSignaling(roomID, c.ID, iceMsg); err != nil {
						log.Printf("[Edge Node] Failed to publish viewer ICE candidate to Redis for Room %s: %v\n", roomID, err)
					} else {
						log.Printf("[Edge Node] Published Viewer %s ICE candidate to Node A via channel signaling.%s.%s\n", c.ID, roomID, c.ID)
					}
					return
				}
			}
		}
		return
	}

	var candidateInit webrtc.ICECandidateInit
	if err := json.Unmarshal(msg.Payload, &candidateInit); err != nil || candidateInit.Candidate == "" {
		// Attempt nested wrapper format: { "candidate": { ... } }
		var wrapped struct {
			Candidate webrtc.ICECandidateInit `json:"candidate"`
		}
		if err2 := json.Unmarshal(msg.Payload, &wrapped); err2 == nil && wrapped.Candidate.Candidate != "" {
			candidateInit = wrapped.Candidate
		} else {
			return
		}
	}

	// Safety check 2: double check state before AddICECandidate
	if pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
		return
	}

	if err := pc.AddICECandidate(candidateInit); err != nil {
		// Silently ignore if connection was closed in the interim
		if pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
			return
		}
		log.Printf("Failed to add ICE candidate for client %s: %v\n", c.ID, err)
		return
	}

	log.Printf("Successfully added ICE candidate for client %s\n", c.ID)
}

// handleSDPAnswer applies a renegotiated remote SDP Answer from a viewer/client
func (c *Client) handleSDPAnswer(msg *models.SignalingMessage) {
	c.mu.Lock()
	pc := c.PeerConnection
	c.mu.Unlock()

	if pc == nil || pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
		log.Printf("Cannot handle SDP answer: PeerConnection closed or nil for client %s\n", c.ID)
		return
	}

	var answer webrtc.SessionDescription
	if err := json.Unmarshal(msg.Payload, &answer); err != nil {
		var sdpWrapper struct {
			SDP  string `json:"sdp"`
			Type string `json:"type"`
		}
		if wrapErr := json.Unmarshal(msg.Payload, &sdpWrapper); wrapErr == nil && sdpWrapper.SDP != "" {
			answer = webrtc.SessionDescription{
				SDP:  sdpWrapper.SDP,
				Type: webrtc.SDPTypeAnswer,
			}
		} else {
			log.Printf("Failed to unmarshal SDP answer from client %s: %v\n", c.ID, err)
			return
		}
	}

	if err := pc.SetRemoteDescription(answer); err != nil {
		log.Printf("Failed to SetRemoteDescription (SDP Answer) for client %s in Room %s: %v\n", c.ID, msg.RoomID, err)
		return
	}
	log.Printf("Successfully applied renegotiated SDP Answer for client %s in Room %s\n", c.ID, msg.RoomID)

	// Send immediate PLI to Host so client receives instant keyframe without frozen video
	if c.RoomManager != nil {
		c.RoomManager.SendPLIToHost(msg.RoomID)
	}
}

// TriggerICERestart generates a new WebRTC offer with ICERestart: true and sends it to the client
func (c *Client) TriggerICERestart(roomID string) {
	c.mu.Lock()
	pc := c.PeerConnection
	c.mu.Unlock()

	if pc == nil || pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
		log.Printf("[ICE Restart] Cannot restart ICE: PeerConnection closed or nil for client %s\n", c.ID)
		return
	}

	if pc.SignalingState() != webrtc.SignalingStateStable {
		log.Printf("[ICE Restart] Cannot restart ICE: PeerConnection signaling state is %s (must be stable)\n", pc.SignalingState().String())
		return
	}

	if roomID == "" {
		roomID = c.RoomID
	}

	log.Printf("[ICE Restart] Generating new WebRTC Offer with ICERestart: true for client %s in Room '%s'\n", c.ID, roomID)

	offer, err := pc.CreateOffer(&webrtc.OfferOptions{
		ICERestart: true,
	})
	if err != nil {
		log.Printf("[ICE Restart] Failed to create ICE restart offer for client %s: %v\n", c.ID, err)
		return
	}

	if err := pc.SetLocalDescription(offer); err != nil {
		log.Printf("[ICE Restart] Failed to set local description (ICE restart) for client %s: %v\n", c.ID, err)
		return
	}

	offerJSON, err := json.Marshal(offer)
	if err != nil {
		log.Printf("[ICE Restart] Failed to marshal ICE restart offer for client %s: %v\n", c.ID, err)
		return
	}

	restartMsg := &models.SignalingMessage{
		Action:  "offer",
		Event:   "offer",
		RoomID:  roomID,
		UserID:  c.ID,
		Payload: offerJSON,
	}

	if encoded, err := restartMsg.Encode(); err == nil {
		c.SafeSend(encoded)
		log.Printf("[ICE Restart] Dispatched ICE restart offer to client %s for Room %s\n", c.ID, roomID)
	}
}

// handleICERestartRequest handles an explicit ice_restart request sent from the client SDK
func (c *Client) handleICERestartRequest(msg *models.SignalingMessage) {
	roomID := msg.RoomID
	if roomID == "" {
		roomID = c.RoomID
	}
	log.Printf("[ICE Restart] Received client-initiated ICE restart request from %s in Room %s\n", c.ID, roomID)
	c.TriggerICERestart(roomID)
}

// SafeSend sends message bytes to client.Send channel without panicking if channel is closed or full
func (c *Client) SafeSend(data []byte) (sent bool) {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return false
	}

	defer func() {
		if r := recover(); r != nil {
			sent = false
		}
	}()

	select {
	case c.Send <- data:
		return true
	default:
		return false
	}
}

// CloseSend safely closes the Send channel exactly once
func (c *Client) CloseSend() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.closed {
		c.closed = true
		if c.Send != nil {
			close(c.Send)
		}
	}
}

// WriteMessage safely writes a message to the WebSocket connection protected by writeMu
func (c *Client) WriteMessage(messageType int, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.Conn == nil {
		return errors.New("websocket connection is nil")
	}
	_ = c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
	return c.Conn.WriteMessage(messageType, data)
}

// WriteJSON safely writes a JSON payload to the WebSocket connection protected by writeMu
func (c *Client) WriteJSON(v any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.Conn == nil {
		return errors.New("websocket connection is nil")
	}
	_ = c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
	return c.Conn.WriteJSON(v)
}

// WritePump pumps messages from the send channel to the websocket connection.
// Sends periodic ping messages to keep connection alive and detect dropped clients.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.writeMu.Lock()
		if c.Conn != nil {
			_ = c.Conn.Close()
		}
		c.writeMu.Unlock()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.writeMu.Lock()
			if c.Conn == nil {
				c.writeMu.Unlock()
				return
			}
			_ = c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				_ = c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				c.writeMu.Unlock()
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				c.writeMu.Unlock()
				return
			}
			if _, err := w.Write(message); err != nil {
				c.writeMu.Unlock()
				return
			}

			// Add queued messages to the current websocket frame
			n := len(c.Send)
			for i := 0; i < n; i++ {
				if _, err := w.Write([]byte{'\n'}); err != nil {
					c.writeMu.Unlock()
					return
				}
				if _, err := w.Write(<-c.Send); err != nil {
					c.writeMu.Unlock()
					return
				}
			}

			if err := w.Close(); err != nil {
				c.writeMu.Unlock()
				return
			}
			c.writeMu.Unlock()

		case <-ticker.C:
			c.writeMu.Lock()
			if c.Conn == nil {
				c.writeMu.Unlock()
				return
			}
			_ = c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.writeMu.Unlock()
				return
			}
			c.writeMu.Unlock()
		}
	}
}

// handlePKRequest forwards a PK battle invitation to the opponent room host
func (c *Client) handlePKRequest(msg *models.SignalingMessage) {
	roomID := msg.RoomID
	targetRoom := msg.TargetUser
	if targetRoom == "" && len(msg.Payload) > 0 {
		var payloadData struct {
			TargetRoom string `json:"target_room"`
			TargetUser string `json:"target_user"`
			RoomB      string `json:"room_b"`
		}
		if err := json.Unmarshal(msg.Payload, &payloadData); err == nil {
			if payloadData.TargetRoom != "" {
				targetRoom = payloadData.TargetRoom
			} else if payloadData.RoomB != "" {
				targetRoom = payloadData.RoomB
			} else if payloadData.TargetUser != "" {
				targetRoom = payloadData.TargetUser
			}
		}
	}

	if targetRoom == "" {
		log.Printf("PK request rejected: missing target_room\n")
		return
	}

	targetRoomObj, exists := c.RoomManager.GetRoom(targetRoom)
	if !exists || targetRoomObj == nil {
		log.Printf("PK request target room %s not found\n", targetRoom)
		return
	}

	// Forward pk_request to target room host
	if hostClient, ok := targetRoomObj.HostClient.(*Client); ok && hostClient != nil {
		reqPayload, _ := json.Marshal(map[string]any{
			"event":        "pk_request",
			"from_room_id": roomID,
			"from_host_id": c.ID,
			"room_a_id":    roomID,
		})
		forwardMsg := &models.SignalingMessage{
			Event:   "pk_request",
			RoomID:  targetRoom,
			UserID:  c.ID,
			Payload: reqPayload,
		}
		if enc, err := forwardMsg.Encode(); err == nil {
			select {
			case hostClient.Send <- enc:
				log.Printf("Relayed pk_request from Room %s to Host of Room %s\n", roomID, targetRoom)
			default:
			}
		}
	}
}

// handlePKAccept accepts a PK battle and launches cross-room cascading & routing
func (c *Client) handlePKAccept(msg *models.SignalingMessage) {
	roomID := msg.RoomID
	targetRoom := msg.TargetUser
	if targetRoom == "" && len(msg.Payload) > 0 {
		var payloadData struct {
			TargetRoom string `json:"target_room"`
			FromRoomID string `json:"from_room_id"`
			RoomAID    string `json:"room_a_id"`
			Room1      string `json:"room_1"`
		}
		if err := json.Unmarshal(msg.Payload, &payloadData); err == nil {
			if payloadData.TargetRoom != "" {
				targetRoom = payloadData.TargetRoom
			} else if payloadData.FromRoomID != "" {
				targetRoom = payloadData.FromRoomID
			} else if payloadData.RoomAID != "" {
				targetRoom = payloadData.RoomAID
			} else if payloadData.Room1 != "" {
				targetRoom = payloadData.Room1
			}
		}
	}

	if targetRoom == "" {
		log.Printf("PK accept rejected: missing target_room\n")
		return
	}

	pkm := c.RoomManager.GetPKManager()
	if pkm == nil {
		log.Printf("PKManager not initialized\n")
		return
	}

	if err := pkm.StartPK(targetRoom, roomID); err != nil {
		log.Printf("Failed to start PK between %s and %s: %v\n", targetRoom, roomID, err)
	}
}

// handlePKStop ends an active PK battle session
func (c *Client) handlePKStop(msg *models.SignalingMessage) {
	roomID := msg.RoomID
	if roomID == "" {
		return
	}

	pkm := c.RoomManager.GetPKManager()
	if pkm == nil {
		return
	}

	_ = pkm.StopPK(roomID)
}

// handleKickParticipant handles manual kicking/banning of a participant by the Host
func (c *Client) handleKickParticipant(msg *models.SignalingMessage) {
	roomID := msg.RoomID
	if roomID == "" {
		return
	}

	room, exists := c.RoomManager.GetRoom(roomID)
	if !exists || room == nil {
		return
	}

	// Verify that only the room host (or admin) can kick participants
	if c.ID != room.HostID && c.Role != "admin" {
		log.Printf("Kick participant rejected: client %s is not host or admin of room %s\n", c.ID, roomID)
		return
	}

	var payload struct {
		TargetUserID string `json:"target_user_id"`
		TargetUser   string `json:"target_user"`
		Reason       string `json:"reason"`
	}
	_ = json.Unmarshal(msg.Payload, &payload)
	targetID := payload.TargetUserID
	if targetID == "" {
		targetID = payload.TargetUser
	}
	if targetID == "" {
		targetID = msg.TargetUser
	}

	if targetID == "" {
		log.Printf("Kick participant rejected: missing target_user_id\n")
		return
	}

	// Cannot kick the host
	if targetID == room.HostID {
		return
	}

	appCfg := config.GetAppConfig()
	if appCfg.YAML.Moderation.AutoBanOnKick {
		room.AddBannedUser(targetID)
		log.Printf("[Moderation] User %s banned from room %s\n", targetID, roomID)
	}

	// Cancel any pending reconnect timers
	room.CancelParticipantReconnectTimer(targetID)

	// If target is connected locally, close connection and notify
	if viewerObj, found := room.GetViewer(targetID); found && viewerObj != nil {
		if vc, ok := viewerObj.(*Client); ok && vc != nil {
			kickedPayload, _ := json.Marshal(map[string]any{
				"event":   "participant_removed",
				"reason":  "kicked_by_host",
				"room_id": roomID,
				"user_id": targetID,
			})
			if enc, err := (&models.SignalingMessage{
				Event:   "participant_removed",
				RoomID:  roomID,
				UserID:  targetID,
				Payload: kickedPayload,
			}).Encode(); err == nil {
				select {
				case vc.Send <- enc:
				default:
				}
			}
			vc.mu.Lock()
			if vc.PeerConnection != nil {
				_ = vc.PeerConnection.Close()
				vc.PeerConnection = nil
			}
			if vc.Conn != nil {
				_ = vc.Conn.Close()
			}
			vc.mu.Unlock()
		}
	}

	// Remove from room and SFU tracks
	c.RoomManager.RemoveViewer(roomID, targetID)

	// Broadcast participant_removed to everyone in the room
	removedPayload, _ := json.Marshal(map[string]any{
		"event":   "participant_removed",
		"room_id": roomID,
		"user_id": targetID,
		"reason":  "kicked_by_host",
	})
	_ = c.RoomManager.BroadcastToRoom(roomID, &models.SignalingMessage{
		Event:   "participant_removed",
		RoomID:  roomID,
		UserID:  targetID,
		Payload: removedPayload,
	})
}

// handleEndRoom handles manual room termination (Kill Switch) triggered by Host, Moderator, or Admin
func (c *Client) handleEndRoom(msg *models.SignalingMessage) {
	roomID := msg.RoomID
	if roomID == "" && len(msg.Payload) > 0 {
		var payloadData struct {
			RoomID string `json:"room_id"`
		}
		_ = json.Unmarshal(msg.Payload, &payloadData)
		roomID = payloadData.RoomID
	}
	if roomID == "" {
		roomID = c.RoomID
	}

	if roomID == "" {
		log.Printf("End room rejected: missing room_id\n")
		return
	}

	room, exists := c.RoomManager.GetRoom(roomID)
	if !exists || room == nil {
		log.Printf("End room rejected: room %s does not exist\n", roomID)
		return
	}

	// Authorization check: User must be Room Host, Moderator, or Admin
	isHost := c.ID == room.HostID
	isAdminOrMod := c.Role == "admin" || c.Role == "moderator"
	if c.Claims != nil {
		if c.Claims.Role == "admin" || c.Claims.Role == "moderator" {
			isAdminOrMod = true
		}
	}

	if !isHost && !isAdminOrMod {
		log.Printf("End room rejected: client %s is not authorized to end room %s (Role: %s)\n", c.ID, roomID, c.Role)
		errPayload, _ := json.Marshal(map[string]any{
			"event":   "error",
			"code":    "unauthorized",
			"message": "Only the Host or a Moderator/Admin can end this room",
			"room_id": roomID,
		})
		if enc, err := (&models.SignalingMessage{
			Event:   "error",
			RoomID:  roomID,
			Payload: errPayload,
		}).Encode(); err == nil {
			select {
			case c.Send <- enc:
			default:
			}
		}
		return
	}

	reason := "closed_by_host"
	if isAdminOrMod && !isHost {
		reason = "closed_by_moderator"
	}

	log.Printf("[Kill Switch] Authorized end_room request for room '%s' by user '%s' (%s)\n", roomID, c.ID, reason)
	c.RoomManager.ForceEndRoom(roomID, c.ID, reason)
}

// handleRequestLayer switches the spatial simulcast layer for a viewer to optimize download bandwidth
func (c *Client) handleRequestLayer(msg *models.SignalingMessage) {
	roomID := msg.RoomID
	if roomID == "" {
		c.mu.Lock()
		roomID = c.RoomID
		c.mu.Unlock()
	}

	room, exists := c.RoomManager.GetRoom(roomID)
	if !exists || room == nil {
		log.Printf("Request layer failed: room %s not found\n", roomID)
		return
	}

	var req struct {
		TargetUser string `json:"target_user"`
		Layer      string `json:"layer"`
		RID        string `json:"rid"`
	}

	if len(msg.Payload) > 0 {
		_ = json.Unmarshal(msg.Payload, &req)
	}

	targetLayer := req.Layer
	if targetLayer == "" {
		targetLayer = req.RID
	}
	if targetLayer != internalWebRTC.LayerHigh && targetLayer != internalWebRTC.LayerMedium && targetLayer != internalWebRTC.LayerLow {
		targetLayer = internalWebRTC.LayerHigh
	}

	// Retrieve TrackSwitcher for this viewer
	if switcherObj, ok := room.GetTrackSwitcher(c.ID); ok && switcherObj != nil {
		if switcher, ok := switcherObj.(*internalWebRTC.TrackSwitcher); ok && switcher != nil {
			switcher.SwitchLayer(targetLayer)
			room.SendPLIImmediate()
			log.Printf("[Dynamic Layer] Switched layer to '%s' for viewer %s in Room %s\n", targetLayer, c.ID, roomID)

			respPayload, _ := json.Marshal(map[string]any{
				"event":       "layer_switched",
				"target_user": req.TargetUser,
				"layer":       targetLayer,
				"status":      "ok",
			})
			respMsg := models.SignalingMessage{
				Event:   "layer_switched",
				RoomID:  roomID,
				UserID:  c.ID,
				Payload: respPayload,
			}
			if enc, err := respMsg.Encode(); err == nil {
				c.SafeSend(enc)
			}
		}
	}
}

// handleSetViewport updates the dynamic viewport and server-side track pausing for a viewer
func (c *Client) handleSetViewport(msg *models.SignalingMessage) {
	roomID := msg.RoomID
	if roomID == "" {
		roomID = c.RoomID
	}
	if roomID == "" || c.RoomManager == nil {
		return
	}

	room, exists := c.RoomManager.GetRoom(roomID)
	if !exists || room == nil {
		return
	}

	var req struct {
		VisibleSpeakers []string `json:"visible_speakers"`
		VisibleCohosts  []string `json:"visible_cohosts"`
		Tracks          []string `json:"tracks"`
	}

	if len(msg.Payload) > 0 {
		_ = json.Unmarshal(msg.Payload, &req)
	}

	visible := req.VisibleSpeakers
	if len(visible) == 0 {
		visible = req.VisibleCohosts
	}
	if len(visible) == 0 {
		visible = req.Tracks
	}

	// Update room's ViewportManager
	if vmAny := room.GetViewportManager(); vmAny != nil {
		if vm, ok := vmAny.(*internalWebRTC.ViewportManager); ok && vm != nil {
			vm.SetVisibleTracks(c.ID, visible)
		}
	} else {
		// Initialize if not already present
		vm := internalWebRTC.NewViewportManager(roomID)
		vm.SetVisibleTracks(c.ID, visible)
		room.SetViewportManager(vm)
	}

	log.Printf("[Signaling Viewport] Viewer %s updated viewport in Room %s: %v\n", c.ID, roomID, visible)

	respPayload, _ := json.Marshal(map[string]any{
		"event":            "viewport_updated",
		"visible_speakers": visible,
		"status":           "ok",
	})
	respMsg := models.SignalingMessage{
		Event:   "viewport_updated",
		RoomID:  roomID,
		UserID:  c.ID,
		Payload: respPayload,
	}
	if enc, err := respMsg.Encode(); err == nil {
		c.SafeSend(enc)
	}
}
