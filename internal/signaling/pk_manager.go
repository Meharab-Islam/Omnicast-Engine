package signaling

import (
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
	"live-media-server/internal/models"
)

// PKSession stores details of a PK battle session between two rooms
type PKSession struct {
	RoomID1   string    `json:"room_id_1"`
	RoomID2   string    `json:"room_id_2"`
	CreatedAt time.Time `json:"created_at"`
}

// PKManager manages live PK battle sessions and cross-routing media streams between rooms
type PKManager struct {
	roomManager *RoomManager
	activePKs   map[string]*PKSession
	mu          sync.RWMutex
}

// NewPKManager creates a new PKManager instance
func NewPKManager(rm *RoomManager) *PKManager {
	return &PKManager{
		roomManager: rm,
		activePKs:   make(map[string]*PKSession),
	}
}

// StartPK establishes cross-routing between Room 1 and Room 2.
// Room 1 Host's tracks are forwarded to Room 2 Viewers, and Room 2 Host's tracks are forwarded to Room 1 Viewers.
func (pkm *PKManager) StartPK(roomID1, roomID2 string) error {
	if roomID1 == "" || roomID2 == "" {
		return errors.New("both roomID1 and roomID2 must be specified")
	}
	if roomID1 == roomID2 {
		return errors.New("cannot start PK battle with the same room")
	}

	pkm.mu.Lock()
	defer pkm.mu.Unlock()

	// Check if any of the rooms are already in a PK session
	if _, exists := pkm.activePKs[roomID1]; exists {
		return errors.New("room1 is already engaged in a PK session")
	}
	if _, exists := pkm.activePKs[roomID2]; exists {
		return errors.New("room2 is already engaged in a PK session")
	}

	// Fetch both rooms from RoomManager
	room1, exists1 := pkm.roomManager.GetRoom(roomID1)
	if !exists1 || room1 == nil {
		return errors.New("room1 not found: " + roomID1)
	}

	room2, exists2 := pkm.roomManager.GetRoom(roomID2)
	if !exists2 || room2 == nil {
		return errors.New("room2 not found: " + roomID2)
	}

	log.Printf("Starting PK battle cross-routing between Room '%s' and Room '%s'...\n", roomID1, roomID2)

	// 1. Cross-route: Forward Room 1's Host tracks to Room 2 Viewers
	for viewerID, v := range room2.Viewers {
		if viewerClient, ok := v.(*Client); ok && viewerClient != nil {
			addTracksToPeer(viewerClient, room1.VideoTrack, room1.AudioTrack)
			log.Printf("Added Room 1 tracks to Viewer %s in Room 2\n", viewerID)
		}
	}

	// 2. Cross-route: Forward Room 2's Host tracks to Room 1 Viewers
	for viewerID, v := range room1.Viewers {
		if viewerClient, ok := v.(*Client); ok && viewerClient != nil {
			addTracksToPeer(viewerClient, room2.VideoTrack, room2.AudioTrack)
			log.Printf("Added Room 2 tracks to Viewer %s in Room 1\n", viewerID)
		}
	}

	// Create and register the PK Session
	session := &PKSession{
		RoomID1:   roomID1,
		RoomID2:   roomID2,
		CreatedAt: time.Now(),
	}
	pkm.activePKs[roomID1] = session
	pkm.activePKs[roomID2] = session

	// Broadcast 'pk_started' signaling event to participants in both rooms
	pkPayload, _ := json.Marshal(map[string]any{
		"room_1":     roomID1,
		"room_2":     roomID2,
		"host_1":     room1.HostID,
		"host_2":     room2.HostID,
		"status":     "active",
		"created_at": session.CreatedAt.Unix(),
	})

	pkm.broadcastToRoom(room1, &models.SignalingMessage{
		Event:   "pk_started",
		RoomID:  roomID1,
		Payload: pkPayload,
	})

	pkm.broadcastToRoom(room2, &models.SignalingMessage{
		Event:   "pk_started",
		RoomID:  roomID2,
		Payload: pkPayload,
	})

	log.Printf("PK battle successfully started between Room '%s' and Room '%s'.\n", roomID1, roomID2)
	return nil
}

// StopPK ends an active PK battle session and notifies participants
func (pkm *PKManager) StopPK(roomID string) error {
	pkm.mu.Lock()
	defer pkm.mu.Unlock()

	session, exists := pkm.activePKs[roomID]
	if !exists {
		return errors.New("no active PK session found for room: " + roomID)
	}

	delete(pkm.activePKs, session.RoomID1)
	delete(pkm.activePKs, session.RoomID2)

	endPayload, _ := json.Marshal(map[string]string{
		"room_1": session.RoomID1,
		"room_2": session.RoomID2,
		"status": "ended",
	})

	if room1, ok := pkm.roomManager.GetRoom(session.RoomID1); ok {
		pkm.broadcastToRoom(room1, &models.SignalingMessage{
			Event:   "pk_ended",
			RoomID:  session.RoomID1,
			Payload: endPayload,
		})
	}

	if room2, ok := pkm.roomManager.GetRoom(session.RoomID2); ok {
		pkm.broadcastToRoom(room2, &models.SignalingMessage{
			Event:   "pk_ended",
			RoomID:  session.RoomID2,
			Payload: endPayload,
		})
	}

	log.Printf("PK battle ended between Room '%s' and Room '%s'.\n", session.RoomID1, session.RoomID2)
	return nil
}

// GetPKSession retrieves the active PK session for a room
func (pkm *PKManager) GetPKSession(roomID string) (*PKSession, bool) {
	pkm.mu.RLock()
	defer pkm.mu.RUnlock()
	session, exists := pkm.activePKs[roomID]
	return session, exists
}

// addTracksToPeer safely adds video and audio tracks to a client's PeerConnection
func addTracksToPeer(client *Client, videoTrack, audioTrack *webrtc.TrackLocalStaticRTP) {
	if client == nil {
		return
	}

	client.mu.Lock()
	pc := client.PeerConnection
	client.mu.Unlock()

	if pc == nil || pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
		return
	}

	if videoTrack != nil {
		if sender, err := pc.AddTrack(videoTrack); err == nil {
			go func() {
				rtcpBuf := make([]byte, 1500)
				for {
					if _, _, rtcpErr := sender.Read(rtcpBuf); rtcpErr != nil {
						return
					}
				}
			}()
		}
	}

	if audioTrack != nil {
		if sender, err := pc.AddTrack(audioTrack); err == nil {
			go func() {
				rtcpBuf := make([]byte, 1500)
				for {
					if _, _, rtcpErr := sender.Read(rtcpBuf); rtcpErr != nil {
						return
					}
				}
			}()
		}
	}
}

// broadcastToRoom sends a signaling message to all viewers in a given room
func (pkm *PKManager) broadcastToRoom(room *models.Room, msg *models.SignalingMessage) {
	if room == nil || msg == nil {
		return
	}

	encoded, err := msg.Encode()
	if err != nil {
		return
	}

	for _, v := range room.Viewers {
		if viewerClient, ok := v.(*Client); ok && viewerClient != nil {
			select {
			case viewerClient.Send <- encoded:
			default:
				log.Printf("Viewer %s buffer full during PK broadcast\n", viewerClient.ID)
			}
		}
	}
}
