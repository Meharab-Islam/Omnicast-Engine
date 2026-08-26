package signaling

import (
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/pion/rtcp"
	pionWebRTC "github.com/pion/webrtc/v3"
	"live-media-server/internal/api"
	"live-media-server/internal/broker"
	"live-media-server/internal/models"
	internalWebRTC "live-media-server/internal/webrtc"
)

// RoomManager manages all active streaming rooms and provides thread-safe operations
type RoomManager struct {
	activeRooms       map[string]*models.Room
	webhookDispatcher *api.WebhookDispatcher
	broker            *broker.RedisBroker
	cascadeManager    *internalWebRTC.CascadeManager
	serverRole        string
	serverAddr        string
	mu                sync.RWMutex
}

// NewRoomManager initializes and returns a new RoomManager instance
func NewRoomManager() *RoomManager {
	return &RoomManager{
		activeRooms: make(map[string]*models.Room),
	}
}

// SetWebhookDispatcher attaches a WebhookDispatcher instance
func (rm *RoomManager) SetWebhookDispatcher(d *api.WebhookDispatcher) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.webhookDispatcher = d
}

// SetBroker attaches a RedisBroker instance and registers distributed Pub/Sub handler
func (rm *RoomManager) SetBroker(b *broker.RedisBroker) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.broker = b
	if b != nil {
		b.SetMessageHandler(func(roomID string, msg *models.SignalingMessage) {
			rm.mu.RLock()
			room, exists := rm.activeRooms[roomID]
			rm.mu.RUnlock()
			if exists && room != nil {
				_ = broadcastToRoomInternal(room, msg)
			}
		})
	}
}

// SetServerConfig configures the role (origin/edge) and public address of this server
func (rm *RoomManager) SetServerConfig(role, publicAddr string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if role == "" {
		role = "origin"
	}
	rm.serverRole = role
	rm.serverAddr = publicAddr
}

// GetServerConfig returns role and public address of this server
func (rm *RoomManager) GetServerConfig() (role, publicAddr string) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.serverRole, rm.serverAddr
}

// SetCascadeManager attaches a CascadeManager instance for Edge nodes
func (rm *RoomManager) SetCascadeManager(cm *internalWebRTC.CascadeManager) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.cascadeManager = cm
}

// GetCascadeManager returns the attached CascadeManager instance
func (rm *RoomManager) GetCascadeManager() *internalWebRTC.CascadeManager {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.cascadeManager
}

// GetBroker returns the attached RedisBroker instance
func (rm *RoomManager) GetBroker() *broker.RedisBroker {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.broker
}

// GetWebhookDispatcher retrieves the attached WebhookDispatcher
func (rm *RoomManager) GetWebhookDispatcher() *api.WebhookDispatcher {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.webhookDispatcher
}

// CreateRoom creates a new Room with the provided hostID in a thread-safe manner
func (rm *RoomManager) CreateRoom(roomID, hostID string) (*models.Room, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.activeRooms[roomID]; exists {
		return nil, errors.New("room already exists")
	}

	room := models.NewRoom(roomID, hostID)
	rm.activeRooms[roomID] = room

	// Subscribe to Redis room channel if broker is active
	if rm.broker != nil && rm.broker.IsActive() {
		_ = rm.broker.SubscribeRoom(roomID)

		// If Origin server, register this room in Redis Global Registry
		if rm.serverRole == "origin" || rm.serverRole == "" {
			_ = rm.broker.RegisterRoomOrigin(roomID, rm.serverAddr, 24*time.Hour)
		}
	}

	// Trigger room_started webhook event
	if rm.webhookDispatcher != nil {
		rm.webhookDispatcher.Dispatch(api.WebhookEvent{
			Event:  api.EventRoomStarted,
			RoomID: roomID,
			UserID: hostID,
			Data: map[string]any{
				"host_id":    hostID,
				"created_at": room.CreatedAt.Format(time.RFC3339),
			},
		})
	}

	return room, nil
}

// JoinViewer registers a viewer client to an existing room in a thread-safe manner and broadcasts viewer count
func (rm *RoomManager) JoinViewer(roomID string, viewerClient *Client) error {
	rm.mu.RLock()
	room, exists := rm.activeRooms[roomID]
	dispatcher := rm.webhookDispatcher
	rm.mu.RUnlock()

	if !exists {
		return errors.New("room not found")
	}

	room.AddViewer(viewerClient.ID, viewerClient)

	// Subscribe to Redis room channel if broker is active on this node
	if rm.broker != nil && rm.broker.IsActive() {
		_ = rm.broker.SubscribeRoom(roomID)
	}

	// Trigger user_joined webhook event
	if dispatcher != nil {
		dispatcher.Dispatch(api.WebhookEvent{
			Event:  api.EventUserJoined,
			RoomID: roomID,
			UserID: viewerClient.ID,
			Data: map[string]any{
				"role":         viewerClient.Role,
				"viewer_count": room.ViewersCount(),
			},
		})
	}

	// Broadcast updated viewer state (total_viewers & viewers_list) to everyone in the room & cluster
	viewerCount := room.ViewersCount()
	viewersList := room.GetViewersList()
	payload, _ := json.Marshal(map[string]any{
		"event":         "viewer_update",
		"room_id":       roomID,
		"total_viewers": viewerCount,
		"count":         viewerCount,
		"viewers_list":  viewersList,
	})
	updateMsg := &models.SignalingMessage{
		Event:        "viewer_update",
		RoomID:       roomID,
		TotalViewers: viewerCount,
		ViewersList:  viewersList,
		Payload:      payload,
	}
	_ = rm.BroadcastToRoom(roomID, updateMsg)

	return nil
}

// RemoveViewer removes a viewer client from a room and broadcasts updated viewer state
func (rm *RoomManager) RemoveViewer(roomID, viewerID string) {
	rm.mu.RLock()
	room, exists := rm.activeRooms[roomID]
	dispatcher := rm.webhookDispatcher
	rm.mu.RUnlock()

	if !exists || room == nil {
		return
	}

	room.RemoveViewer(viewerID)
	room.RemoveCoHostTrack(viewerID)

	if dispatcher != nil {
		dispatcher.Dispatch(api.WebhookEvent{
			Event:  api.EventUserLeft,
			RoomID: roomID,
			UserID: viewerID,
			Data: map[string]any{
				"viewer_count": room.ViewersCount(),
			},
		})
	}

	viewerCount := room.ViewersCount()
	viewersList := room.GetViewersList()
	payload, _ := json.Marshal(map[string]any{
		"event":         "viewer_update",
		"room_id":       roomID,
		"total_viewers": viewerCount,
		"count":         viewerCount,
		"viewers_list":  viewersList,
	})
	updateMsg := &models.SignalingMessage{
		Event:        "viewer_update",
		RoomID:       roomID,
		TotalViewers: viewerCount,
		ViewersList:  viewersList,
		Payload:      payload,
	}
	_ = rm.BroadcastToRoom(roomID, updateMsg)
}

// GetRoom retrieves an active room by roomID in a thread-safe manner
func (rm *RoomManager) GetRoom(roomID string) (*models.Room, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	room, exists := rm.activeRooms[roomID]
	return room, exists
}

// RemoveRoom deletes an active room by roomID and cleans up resources
func (rm *RoomManager) RemoveRoom(roomID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.broker != nil && rm.broker.IsActive() {
		rm.broker.UnsubscribeRoom(roomID)
		_ = rm.broker.RemoveRoomOrigin(roomID)
	}

	if room, exists := rm.activeRooms[roomID]; exists {
		room.SetTracks(nil, nil)
		delete(rm.activeRooms, roomID)
	}
}

// BroadcastToRoom sends a signaling message to the host and all viewers of the specified room in a thread-safe manner
// If a RedisBroker is active, it publishes to Redis Pub/Sub so all server instances broadcast to their local clients.
func (rm *RoomManager) BroadcastToRoom(roomID string, msg *models.SignalingMessage) error {
	rm.mu.RLock()
	room, exists := rm.activeRooms[roomID]
	activeBroker := rm.broker
	rm.mu.RUnlock()

	if !exists || room == nil {
		return errors.New("room not found: " + roomID)
	}

	// If distributed Redis broker is active, publish to Redis channel
	if activeBroker != nil && activeBroker.IsActive() {
		return activeBroker.PublishRoomEvent(roomID, msg)
	}

	// Fallback to local in-memory broadcast
	return broadcastToRoomInternal(room, msg)
}

// broadcastToRoomInternal sends a message to participants without acquiring rm.mu
func broadcastToRoomInternal(room *models.Room, msg *models.SignalingMessage) error {
	if room == nil || msg == nil {
		return nil
	}

	encoded, err := msg.Encode()
	if err != nil {
		return err
	}

	room.RLock()
	defer room.RUnlock()

	// Forward message to Host if connected
	if room.HostClient != nil {
		if hostClient, ok := room.HostClient.(*Client); ok && hostClient != nil {
			select {
			case hostClient.Send <- encoded:
			default:
				log.Printf("Host client %s send buffer full\n", room.HostID)
			}
		}
	}

	// Forward message to all Viewers
	for viewerID, v := range room.Viewers {
		if viewerClient, ok := v.(*Client); ok && viewerClient != nil {
			select {
			case viewerClient.Send <- encoded:
			default:
				log.Printf("Viewer client %s send buffer full\n", viewerID)
			}
		}
	}

	return nil
}

// AddTrackAndRenegotiate adds a new media track (e.g. Co-host video/audio) to all connected viewers and the main host in the room,
// and sends renegotiated SDP offers over WebSocket.
func (rm *RoomManager) AddTrackAndRenegotiate(roomID string, track *pionWebRTC.TrackLocalStaticRTP, coHostID string) {
	rm.mu.RLock()
	room, exists := rm.activeRooms[roomID]
	rm.mu.RUnlock()

	if !exists || room == nil || track == nil {
		return
	}

	room.RLock()
	defer room.RUnlock()

	renegotiateClient := func(targetClient any, targetID string) {
		client, ok := targetClient.(*Client)
		if !ok || client == nil || client.ID == coHostID {
			return
		}

		client.mu.Lock()
		pc := client.PeerConnection
		client.mu.Unlock()

		if pc == nil || pc.ConnectionState() == pionWebRTC.PeerConnectionStateClosed {
			return
		}

		// Add new track to target's peer connection
		if _, err := pc.AddTrack(track); err != nil {
			log.Printf("Failed to AddTrack during renegotiation for client %s (Room %s): %v\n", targetID, roomID, err)
			return
		}

		// Create new SDP offer for renegotiation
		offer, err := pc.CreateOffer(nil)
		if err != nil {
			log.Printf("Failed to CreateOffer during renegotiation for client %s (Room %s): %v\n", targetID, roomID, err)
			return
		}

		// Set local description with the new offer
		if err := pc.SetLocalDescription(offer); err != nil {
			log.Printf("Failed to SetLocalDescription during renegotiation for client %s (Room %s): %v\n", targetID, roomID, err)
			return
		}

		// Send sdp_offer signaling message to client
		offerJSON, err := json.Marshal(offer)
		if err != nil {
			return
		}

		offerMsg := models.SignalingMessage{
			Event:   "sdp_offer",
			RoomID:  roomID,
			UserID:  targetID,
			Payload: offerJSON,
		}
		if encoded, err := offerMsg.Encode(); err == nil {
			select {
			case client.Send <- encoded:
				log.Printf("Sent renegotiation SDP offer to client %s for CoHost %s track in Room %s\n", targetID, coHostID, roomID)
			default:
				log.Printf("Client %s send buffer full during renegotiation\n", targetID)
			}
		}
	}

	// Renegotiate with Host so Host can see CoHost
	if room.HostClient != nil {
		renegotiateClient(room.HostClient, room.HostID)
	}

	// Renegotiate with all Viewers
	for viewerID, v := range room.Viewers {
		renegotiateClient(v, viewerID)
	}

	// Force immediate PLI to Host so all viewers receive a fresh keyframe on renegotiation
	hostPC := room.GetHostPeerConnection()
	ssrc := room.GetHostVideoSSRC()
	if hostPC != nil && ssrc != 0 && hostPC.ConnectionState() != pionWebRTC.PeerConnectionStateClosed {
		_ = hostPC.WriteRTCP([]rtcp.Packet{
			&rtcp.PictureLossIndication{MediaSSRC: ssrc},
		})
		log.Printf("[Force PLI] Dispatched keyframe request to Host for Room: %s (SSRC: %d)\n", roomID, ssrc)
	}
}

// SendPLIToHost sends an immediate Picture Loss Indication (Keyframe request) to the Room's Host
func (rm *RoomManager) SendPLIToHost(roomID string) {
	rm.mu.RLock()
	room, exists := rm.activeRooms[roomID]
	rm.mu.RUnlock()

	if !exists || room == nil {
		return
	}

	hostPC := room.GetHostPeerConnection()
	ssrc := room.GetHostVideoSSRC()
	if hostPC != nil && ssrc != 0 && hostPC.ConnectionState() != pionWebRTC.PeerConnectionStateClosed {
		_ = hostPC.WriteRTCP([]rtcp.Packet{
			&rtcp.PictureLossIndication{MediaSSRC: ssrc},
		})
		log.Printf("[Force PLI] Dispatched keyframe request to Host for Room: %s (SSRC: %d)\n", roomID, ssrc)
	}
}

// HandleClientDisconnect handles cleanup and notification when a Host or Viewer disconnects
func (rm *RoomManager) HandleClientDisconnect(client *Client) {
	if client == nil {
		return
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()


	for roomID, room := range rm.activeRooms {
		// If the disconnecting client is the Host of this room
		if room.HostID == client.ID {
			log.Printf("Host client %s disconnected from Room '%s'. Starting 25s Grace Period for reconnection...\n", client.ID, roomID)

			// Clear active host client reference
			room.SetHostClient(nil)
			room.SetHostPeerConnection(nil)

			// Broadcast 'host_reconnecting' event to all viewers in the room
			reconnectingMsg := &models.SignalingMessage{
				Event:  "host_reconnecting",
				RoomID: roomID,
				UserID: client.ID,
			}
			_ = broadcastToRoomInternal(room, reconnectingMsg)

			// Start Grace Period timer (25 seconds) before closing room
			targetRoomID := roomID
			targetHostID := client.ID
			room.StartReconnectTimer(25*time.Second, func() {
				rm.CloseRoomAndNotify(targetRoomID, targetHostID)
			})
		} else {
			// If the disconnecting client is a Viewer or Co-host in this room
			if _, exists := room.GetViewer(client.ID); exists {
				log.Printf("Viewer %s disconnected from room '%s'. Removing from room viewers...\n", client.ID, roomID)
				room.RemoveViewer(client.ID)
				room.RemoveCoHostTrack(client.ID)

				// Trigger user_left webhook event
				if rm.webhookDispatcher != nil {
					rm.webhookDispatcher.Dispatch(api.WebhookEvent{
						Event:  api.EventUserLeft,
						RoomID: roomID,
						UserID: client.ID,
						Data: map[string]any{
							"viewer_count": room.ViewersCount(),
						},
					})
				}

				// Broadcast updated viewer state (total_viewers & viewers_list) to everyone in the room & cluster
				viewerCount := room.ViewersCount()
				viewersList := room.GetViewersList()
				payload, _ := json.Marshal(map[string]any{
					"event":         "viewer_update",
					"room_id":       roomID,
					"total_viewers": viewerCount,
					"count":         viewerCount,
					"viewers_list":  viewersList,
				})
				updateMsg := &models.SignalingMessage{
					Event:        "viewer_update",
					RoomID:       roomID,
					TotalViewers: viewerCount,
					ViewersList:  viewersList,
					Payload:      payload,
				}
				_ = broadcastToRoomInternal(room, updateMsg)
				if rm.broker != nil && rm.broker.IsActive() {
					_ = rm.broker.PublishRoomEvent(roomID, updateMsg)
				}
			}
		}
	}


	// Close client's PeerConnection to release WebRTC resources and prevent memory leak
	client.mu.Lock()
	if client.PeerConnection != nil {
		_ = client.PeerConnection.Close()
		client.PeerConnection = nil
		log.Printf("Closed PeerConnection for client %s\n", client.ID)
	}
	client.mu.Unlock()
}

// CloseRoomAndNotify closes an active room, notifies viewers with 'room_closed', triggers webhook, and cleans up resources
func (rm *RoomManager) CloseRoomAndNotify(roomID, hostID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	room, exists := rm.activeRooms[roomID]
	if !exists || room == nil {
		return
	}

	log.Printf("Closing Room '%s' (Host: %s) and notifying viewers...\n", roomID, hostID)

	// Clean up Redis registration and subscription
	if rm.broker != nil && rm.broker.IsActive() {
		rm.broker.UnsubscribeRoom(roomID)
		_ = rm.broker.RemoveRoomOrigin(roomID)
	}

	// Trigger room_ended webhook event
	if rm.webhookDispatcher != nil {
		rm.webhookDispatcher.Dispatch(api.WebhookEvent{
			Event:  api.EventRoomEnded,
			RoomID: roomID,
			UserID: hostID,
			Data: map[string]any{
				"host_id":    hostID,
				"host_score": room.GetHostScore(),
			},
		})
	}

	// Prepare 'room_closed' message for all viewers
	closedMsg := models.SignalingMessage{
		Event:  "room_closed",
		RoomID: roomID,
		UserID: hostID,
	}
	closedBytes, _ := closedMsg.Encode()

	// Broadcast 'room_closed' to all viewers and clear viewers map
	for viewerID, v := range room.Viewers {
		if viewerClient, ok := v.(*Client); ok && viewerClient != nil {
			select {
			case viewerClient.Send <- closedBytes:
			default:
				log.Printf("Viewer %s send buffer full while sending room_closed\n", viewerID)
			}
		}
		room.RemoveViewer(viewerID)
	}

	// Clear media tracks
	room.SetTracks(nil, nil)
	delete(rm.activeRooms, roomID)
	log.Printf("Room '%s' successfully closed and removed.\n", roomID)
}

// RoomInfo represents public room information for the active room list
type RoomInfo struct {
	RoomID      string `json:"room_id"`
	RoomName    string `json:"room_name"`
	HostID      string `json:"host_id"`
	ViewerCount int    `json:"viewer_count"`
}

// GetAllRooms returns a slice of all active rooms containing room_id, room_name, host_id, and viewer_count (thread-safe)
func (rm *RoomManager) GetAllRooms() []RoomInfo {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	rooms := make([]RoomInfo, 0, len(rm.activeRooms))
	for _, room := range rm.activeRooms {
		rooms = append(rooms, RoomInfo{
			RoomID:      room.RoomID,
			RoomName:    room.GetRoomName(),
			HostID:      room.HostID,
			ViewerCount: room.ViewersCount(),
		})
	}
	return rooms
}

// RoomSummary provides clean JSON-serializable info of an active room
type RoomSummary struct {
	RoomID       string `json:"room_id"`
	RoomName     string `json:"room_name"`
	HostID       string `json:"host_id"`
	MainSeatID   string `json:"main_seat_id"`
	HostScore    int    `json:"host_score"`
	CreatedAt    string `json:"created_at"`
	ViewersCount int    `json:"viewers_count"`
}

// GetAllRoomsSummary returns summary list of all active rooms (thread-safe)
func (rm *RoomManager) GetAllRoomsSummary() []RoomSummary {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	summaries := make([]RoomSummary, 0, len(rm.activeRooms))
	for _, room := range rm.activeRooms {
		summaries = append(summaries, RoomSummary{
			RoomID:       room.RoomID,
			RoomName:     room.GetRoomName(),
			HostID:       room.HostID,
			MainSeatID:   room.GetMainSeatID(),
			HostScore:    room.GetHostScore(),
			CreatedAt:    room.CreatedAt.Format(time.RFC3339),
			ViewersCount: room.ViewersCount(),
		})
	}
	return summaries
}

// GetRoomSummary returns summary detail of a specific room (thread-safe)
func (rm *RoomManager) GetRoomSummary(roomID string) (*RoomSummary, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	room, exists := rm.activeRooms[roomID]
	if !exists || room == nil {
		return nil, false
	}

	return &RoomSummary{
		RoomID:       room.RoomID,
		RoomName:     room.GetRoomName(),
		HostID:       room.HostID,
		MainSeatID:   room.GetMainSeatID(),
		HostScore:    room.GetHostScore(),
		CreatedAt:    room.CreatedAt.Format(time.RFC3339),
		ViewersCount: room.ViewersCount(),
	}, true
}

// ActiveRoomsCount returns the total number of currently active rooms
func (rm *RoomManager) ActiveRoomsCount() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.activeRooms)
}
