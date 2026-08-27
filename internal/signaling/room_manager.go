package signaling

import (
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/pion/rtcp"
	pionWebRTC "github.com/pion/webrtc/v3"
	"omnicast/internal/api"
	"omnicast/internal/broker"
	"omnicast/internal/config"
	"omnicast/internal/models"
	internalWebRTC "omnicast/internal/webrtc"
)

// RoomManager manages all active streaming rooms and provides thread-safe operations
type RoomManager struct {
	activeRooms       map[string]*models.Room
	webhookDispatcher *api.WebhookDispatcher
	broker            *broker.RedisBroker
	cascadeManager    *internalWebRTC.CascadeManager
	pkManager         *PKManager
	workerPool        *WorkerPool
	serverRole        string
	serverAddr        string
	pendingScores     map[string]int64
	pendingScoreMu    sync.Mutex
	mu                sync.RWMutex
}

// NewRoomManager initializes and returns a new RoomManager instance with WorkerPool, batch flusher, and TTL refresher
func NewRoomManager() *RoomManager {
	rm := &RoomManager{
		activeRooms:   make(map[string]*models.Room),
		workerPool:    NewWorkerPool(64, 32768),
		pendingScores: make(map[string]int64),
	}
	rm.startBatchFlusher()
	rm.startTTLRefresherWorker()
	return rm
}

// startBatchFlusher starts a background goroutine to flush accumulated gift scores and debounced state to Redis every 2s
func (rm *RoomManager) startBatchFlusher() {
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			rm.flushPendingScores()
		}
	}()
}

// startTTLRefresherWorker periodically refreshes the 24-hour TTL in Redis for all active rooms (every 30m)
func (rm *RoomManager) startTTLRefresherWorker() {
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			rm.refreshActiveRoomsTTL()
		}
	}()
}

// refreshActiveRoomsTTL collects all active room IDs and extends their 24h expiration in Redis
func (rm *RoomManager) refreshActiveRoomsTTL() {
	rm.mu.RLock()
	if rm.broker == nil || !rm.broker.IsActive() || len(rm.activeRooms) == 0 {
		rm.mu.RUnlock()
		return
	}

	roomIDs := make([]string, 0, len(rm.activeRooms))
	for id := range rm.activeRooms {
		roomIDs = append(roomIDs, id)
	}
	activeBroker := rm.broker
	rm.mu.RUnlock()

	if err := activeBroker.BatchRefreshRoomTTLs(nil, roomIDs); err != nil {
		log.Printf("[Redis Warning] Failed to refresh TTL for active rooms: %v\n", err)
	} else {
		log.Printf("[Redis TTL Refresher] Refreshed 24h TTL for %d active rooms.\n", len(roomIDs))
	}
}

// flushPendingScores flushes in-memory score deltas to Redis using pipeline INCRBY
func (rm *RoomManager) flushPendingScores() {
	rm.pendingScoreMu.Lock()
	if len(rm.pendingScores) == 0 {
		rm.pendingScoreMu.Unlock()
		return
	}
	deltas := make(map[string]int64, len(rm.pendingScores))
	for k, v := range rm.pendingScores {
		deltas[k] = v
	}
	rm.pendingScores = make(map[string]int64)
	rm.pendingScoreMu.Unlock()

	rm.mu.RLock()
	activeBroker := rm.broker
	rm.mu.RUnlock()

	if activeBroker != nil && activeBroker.IsActive() {
		_ = activeBroker.BatchIncrementScores(nil, deltas)
	}
}

// SetWebhookDispatcher attaches a WebhookDispatcher instance
func (rm *RoomManager) SetWebhookDispatcher(d *api.WebhookDispatcher) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.webhookDispatcher = d
}

// SetPKManager attaches a PKManager instance
func (rm *RoomManager) SetPKManager(pkm *PKManager) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.pkManager = pkm
}

// GetPKManager retrieves the attached PKManager
func (rm *RoomManager) GetPKManager() *PKManager {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.pkManager
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

	// Synchronize initial RoomState to Redis without re-locking rm.mu
	syncRoomStateInternal(room, rm.broker)

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

	// Cancel any pending empty room auto-destruction timer
	room.CancelEmptyRoomTimer()

	// If participant was in reconnecting grace period, restore state and broadcast 'participant_reconnected'
	if room.CancelParticipantReconnectTimer(viewerClient.ID) {
		log.Printf("Participant %s reconnected to Room '%s' within grace period!\n", viewerClient.ID, roomID)
		reconnPayload, _ := json.Marshal(map[string]any{
			"event":   "participant_reconnected",
			"room_id": roomID,
			"user_id": viewerClient.ID,
			"data": map[string]any{
				"user_id": viewerClient.ID,
			},
		})
		_ = rm.BroadcastToRoom(roomID, &models.SignalingMessage{
			Event:   "participant_reconnected",
			RoomID:  roomID,
			UserID:  viewerClient.ID,
			Payload: reconnPayload,
		})
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

	// Sync updated viewer count and seats to Redis RoomState
	rm.SyncRoomState(roomID)

	// Empty Room Auto-Destroy Check (The '0' Rule)
	if viewerCount == 0 && room.GetHostClient() == nil {
		appCfg := config.GetAppConfig()
		emptyTimeout := appCfg.YAML.RoomManagement.EmptyRoomTimeoutSec
		if emptyTimeout > 0 {
			log.Printf("Room %s is empty. Starting %d-second destruction timer...\n", roomID, emptyTimeout)
			room.StartEmptyRoomTimer(time.Duration(emptyTimeout)*time.Second, func() {
				log.Printf("Empty room %s timeout reached. Executing instant cleanup...\n", roomID)
				rm.destroyRoomInstant(roomID)
			})
		} else {
			log.Printf("Room %s is empty. (empty_room_timeout_sec=0 -> Keeping room alive permanently).\n", roomID)
		}
	}

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

	// Sync updated viewer state to Redis RoomState
	rm.SyncRoomState(roomID)
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
		_ = rm.broker.DeleteRoomState(nil, roomID)
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

	// Collect Viewers to avoid holding lock while sending
	targets := make([]*Client, 0, len(room.Viewers))
	for _, v := range room.Viewers {
		if viewerClient, ok := v.(*Client); ok && viewerClient != nil {
			targets = append(targets, viewerClient)
		}
	}
	room.RUnlock()

	// Forward message to all Viewers
	for _, client := range targets {
		select {
		case client.Send <- encoded:
		default:
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

// RemoveTrackAndRenegotiate removes a Co-Host's media track from the room and updates seats & Redis
func (rm *RoomManager) RemoveTrackAndRenegotiate(roomID, coHostID string) {
	rm.mu.RLock()
	room, exists := rm.activeRooms[roomID]
	activeBroker := rm.broker
	rm.mu.RUnlock()

	if !exists || room == nil {
		return
	}

	room.RemoveCoHostTrack(coHostID)
	room.RemoveUserFromSeats(coHostID)
	syncRoomStateInternal(room, activeBroker)

	// Send PLI to Host to keep streams clean
	rm.SendPLIToHost(roomID)
	log.Printf("[Seat Management] Removed CoHost %s tracks & seat from Room %s\n", coHostID, roomID)
}

// SendPLIToHost sends an immediate Picture Loss Indication (Keyframe request) to the Room's Host with debouncing
func (rm *RoomManager) SendPLIToHost(roomID string) {
	rm.mu.RLock()
	room, exists := rm.activeRooms[roomID]
	rm.mu.RUnlock()

	if !exists || room == nil {
		return
	}

	room.SendPLIThrottled(1500 * time.Millisecond)
}

// HandleClientDisconnect handles cleanup and notification when a Host or Viewer disconnects
func (rm *RoomManager) HandleClientDisconnect(client *Client) {
	if client == nil {
		return
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()


	appCfg := config.GetAppConfig()
	hostGraceSec := appCfg.YAML.RoomManagement.HostGracePeriodSec
	if hostGraceSec <= 0 {
		hostGraceSec = 25
	}
	viewerGraceSec := appCfg.YAML.Moderation.ViewerGracePeriodSec
	if viewerGraceSec <= 0 {
		viewerGraceSec = 120
	}

	for roomID, room := range rm.activeRooms {
		// If the disconnecting client is the Host of this room
		if room.HostID == client.ID {
			log.Printf("Host client %s disconnected from Room '%s'. Starting %ds Grace Period for reconnection...\n", client.ID, roomID, hostGraceSec)

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

			// Start Host Grace Period timer before closing room
			targetRoomID := roomID
			targetHostID := client.ID
			room.StartReconnectTimer(time.Duration(hostGraceSec)*time.Second, func() {
				rm.CloseRoomAndNotify(targetRoomID, targetHostID)
			})
		} else {
			// If the disconnecting client is a Viewer or Co-host in this room
			if _, exists := room.GetViewer(client.ID); exists {
				log.Printf("Participant %s connection dropped from room '%s'. Entering %ds reconnecting state...\n", client.ID, roomID, viewerGraceSec)

				// Broadcast 'participant_reconnecting' event to the room
				reconnPayload, _ := json.Marshal(map[string]any{
					"event":   "participant_reconnecting",
					"room_id": roomID,
					"user_id": client.ID,
					"role":    client.Role,
					"data": map[string]any{
						"user_id": client.ID,
						"role":    client.Role,
					},
				})
				_ = broadcastToRoomInternal(room, &models.SignalingMessage{
					Event:   "participant_reconnecting",
					RoomID:  roomID,
					UserID:  client.ID,
					Payload: reconnPayload,
				})

				// Start participant reconnect timer instead of instant kick
				targetRoomID := roomID
				targetUserID := client.ID
				room.StartParticipantReconnectTimer(targetUserID, time.Duration(viewerGraceSec)*time.Second, func() {
					log.Printf("Participant %s reconnect grace period expired in room %s. Performing cleanup...\n", targetUserID, targetRoomID)
					rm.RemoveViewer(targetRoomID, targetUserID)
				})
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

// destroyRoomInstant completely wipes a room, its Redis keys (UNLINK), PeerConnections, WebSockets, and memory
func (rm *RoomManager) destroyRoomInstant(roomID string) {
	rm.CloseRoomAndNotifyWithReason(roomID, "", "closed_and_destroyed")
}

// ForceEndRoom forcefully terminates and destroys a room, executing full memory, WebRTC, WebSocket, and Redis cleanup
func (rm *RoomManager) ForceEndRoom(roomID, actorID, reason string) {
	if reason == "" {
		reason = "closed_by_host"
	}
	rm.CloseRoomAndNotifyWithReason(roomID, actorID, reason)
}

// CloseRoomAndNotify closes an active room and notifies all participants with default reason
func (rm *RoomManager) CloseRoomAndNotify(roomID, hostID string) {
	rm.CloseRoomAndNotifyWithReason(roomID, hostID, "closed_by_host")
}

// CloseRoomAndNotifyWithReason closes an active room, notifies participants with 'room_ended',
// forcefully closes all WebRTC PeerConnections & WebSockets, flushes Redis with UNLINK, and frees memory.
func (rm *RoomManager) CloseRoomAndNotifyWithReason(roomID, hostID, reason string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	room, exists := rm.activeRooms[roomID]
	if !exists || room == nil {
		return
	}

	if hostID == "" {
		hostID = room.HostID
	}

	log.Printf("[Instant Cleanup] Force-closing Room '%s' (Triggered by: %s, Reason: %s)...\n", roomID, hostID, reason)

	// Cancel any active timers on the room
	room.CancelReconnectTimer()
	room.CancelEmptyRoomTimer()

	// Instant Redis Wipe: Unlink state, score, chats, participants, banned, and PK sessions
	if rm.broker != nil && rm.broker.IsActive() {
		rm.broker.UnsubscribeRoom(roomID)
		_ = rm.broker.RemoveRoomOrigin(roomID)
		_ = rm.broker.UnlinkAllRoomKeys(nil, roomID)
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
				"reason":     reason,
			},
		})
	}

	// Prepare 'room_ended' and 'room_closed' event payloads
	endedPayload, _ := json.Marshal(map[string]any{
		"event":   "room_ended",
		"room_id": roomID,
		"reason":  reason,
		"data": map[string]any{
			"reason":  reason,
			"room_id": roomID,
		},
	})
	endedMsg := &models.SignalingMessage{
		Event:   "room_ended",
		RoomID:  roomID,
		UserID:  hostID,
		Payload: endedPayload,
	}
	endedBytes, _ := endedMsg.Encode()

	// 1. Forcefully terminate and disconnect all Viewers & Co-Hosts
	for viewerID, v := range room.Viewers {
		if viewerClient, ok := v.(*Client); ok && viewerClient != nil {
			select {
			case viewerClient.Send <- endedBytes:
			default:
			}

			// Forcefully close WebRTC and WebSocket
			viewerClient.mu.Lock()
			if viewerClient.PeerConnection != nil {
				_ = viewerClient.PeerConnection.Close()
				viewerClient.PeerConnection = nil
			}
			if viewerClient.Conn != nil {
				_ = viewerClient.Conn.Close()
			}
			viewerClient.mu.Unlock()
		}
		room.RemoveViewer(viewerID)
	}

	// 2. Forcefully terminate Host connection if active
	if room.HostClient != nil {
		if hostClient, ok := room.HostClient.(*Client); ok && hostClient != nil {
			select {
			case hostClient.Send <- endedBytes:
			default:
			}

			hostClient.mu.Lock()
			if hostClient.PeerConnection != nil {
				_ = hostClient.PeerConnection.Close()
				hostClient.PeerConnection = nil
			}
			if hostClient.Conn != nil {
				_ = hostClient.Conn.Close()
			}
			hostClient.mu.Unlock()
		}
	}

	// Clear media tracks & switchers
	room.SetTracks(nil, nil)
	delete(rm.activeRooms, roomID)
	log.Printf("[Kill Switch] Room '%s' completely wiped from memory and network.\n", roomID)
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

// syncRoomStateInternal writes the room state snapshot to Redis without acquiring rm.mu
func syncRoomStateInternal(room *models.Room, activeBroker *broker.RedisBroker) {
	if room == nil {
		return
	}
	state := room.GetRoomState()
	if activeBroker != nil && activeBroker.IsActive() {
		_ = activeBroker.SaveRoomState(nil, state)
	}
}

// SyncRoomState saves the latest RoomState snapshot of a room to Redis
func (rm *RoomManager) SyncRoomState(roomID string) {
	rm.mu.RLock()
	room, exists := rm.activeRooms[roomID]
	activeBroker := rm.broker
	rm.mu.RUnlock()

	if exists && room != nil {
		syncRoomStateInternal(room, activeBroker)
	}
}

// GetRoomState fetches the current RoomState from Redis (with in-memory fallback)
func (rm *RoomManager) GetRoomState(roomID string) *models.RoomState {
	rm.mu.RLock()
	room, exists := rm.activeRooms[roomID]
	activeBroker := rm.broker
	rm.mu.RUnlock()

	// 1. Try fetching from Redis first
	if activeBroker != nil && activeBroker.IsActive() {
		if state, err := activeBroker.GetRoomState(nil, roomID); err == nil && state != nil {
			return state
		}
	}

	// 2. Fallback to in-memory room state
	if exists && room != nil {
		return room.GetRoomState()
	}

	return nil
}

// AddGiftScore atomically updates the gift score in memory and batches increments for Redis pipeline flushing
func (rm *RoomManager) AddGiftScore(roomID string, coins int64) int64 {
	rm.mu.RLock()
	room, exists := rm.activeRooms[roomID]
	rm.mu.RUnlock()

	var newScore int64
	if exists && room != nil {
		newScore = int64(room.AddHostScore(int(coins)))
	}

	// Buffer score delta in memory for periodic Redis INCRBY pipeline flush (Pillar 4)
	rm.pendingScoreMu.Lock()
	rm.pendingScores[roomID] += coins
	rm.pendingScoreMu.Unlock()

	// If the room is engaged in a live PK battle, sync scores across both rooms
	rm.mu.RLock()
	pkm := rm.pkManager
	rm.mu.RUnlock()
	if pkm != nil {
		pkm.SyncPKScore(roomID, newScore)
	}

	return newScore
}

// SetMediaState updates user's media mute state in the room and synchronizes to Redis
func (rm *RoomManager) SetMediaState(roomID, userID string, state models.MediaState) {
	rm.mu.RLock()
	room, exists := rm.activeRooms[roomID]
	rm.mu.RUnlock()

	if exists && room != nil {
		room.SetMediaState(userID, state)
		rm.SyncRoomState(roomID)
	}
}

// ActiveRoomsCount returns the total number of currently active rooms
func (rm *RoomManager) ActiveRoomsCount() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.activeRooms)
}
