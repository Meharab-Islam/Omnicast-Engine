package signaling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"omnicast/internal/models"
)

// PKManager manages live PK battle sessions, cross-room media track distribution, and real-time score syncing
type PKManager struct {
	roomManager *RoomManager
	activePKs   map[string]*models.PKSession
	mu          sync.RWMutex
}

// NewPKManager creates a new PKManager instance
func NewPKManager(rm *RoomManager) *PKManager {
	return &PKManager{
		roomManager: rm,
		activePKs:   make(map[string]*models.PKSession),
	}
}

// StartPK establishes cross-room routing and WebRTC renegotiation between Room 1 and Room 2.
// Host 1's media tracks are forwarded to Room 2, and Host 2's media tracks are forwarded to Room 1.
func (pkm *PKManager) StartPK(roomID1, roomID2 string) error {
	if roomID1 == "" || roomID2 == "" {
		return errors.New("both roomID1 and roomID2 must be specified")
	}
	if roomID1 == roomID2 {
		return errors.New("cannot start PK battle with the same room")
	}

	pkm.mu.Lock()
	defer pkm.mu.Unlock()

	// Check if any room is already in a PK session
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

	// Trigger Edge-to-Origin cascading logic if rooms are distributed across cluster nodes
	if cascadeMgr := pkm.roomManager.GetCascadeManager(); cascadeMgr != nil {
		_ = cascadeMgr.EnsureCascaded(room1)
		_ = cascadeMgr.EnsureCascaded(room2)
	}

	log.Printf("[PK Battle] Starting cross-room routing between Room '%s' (Host: %s) and Room '%s' (Host: %s)...\n", roomID1, room1.HostID, roomID2, room2.HostID)

	// 1. Cross-route Host 1's video track into Room 2 viewers & host with dynamic renegotiation
	if track1 := room1.GetDefaultViewerVideoTrack(); track1 != nil {
		pkm.roomManager.AddTrackAndRenegotiate(roomID2, track1, "pk-"+room1.HostID)
		log.Printf("[PK Battle] Injected Room 1 Host track into Room 2 viewers & host\n")
	}

	// 2. Cross-route Host 2's video track into Room 1 viewers & host with dynamic renegotiation
	if track2 := room2.GetDefaultViewerVideoTrack(); track2 != nil {
		pkm.roomManager.AddTrackAndRenegotiate(roomID1, track2, "pk-"+room2.HostID)
		log.Printf("[PK Battle] Injected Room 2 Host track into Room 1 viewers & host\n")
	}

	// Create and register PK session
	sessionID := fmt.Sprintf("%s_%s", roomID1, roomID2)
	session := &models.PKSession{
		SessionID: sessionID,
		RoomID1:   roomID1,
		RoomID2:   roomID2,
		HostID1:   room1.HostID,
		HostID2:   room2.HostID,
		Score1:    int64(room1.GetHostScore()),
		Score2:    int64(room2.GetHostScore()),
		Status:    "active",
		CreatedAt: time.Now().UTC(),
	}

	pkm.activePKs[roomID1] = session
	pkm.activePKs[roomID2] = session

	// Update Room.PKState on both rooms for instantaneous room_state sync
	room1.SetPKState(&models.PKState{
		IsActive:       true,
		SessionID:      sessionID,
		OpponentID:     room2.HostID,
		OpponentRoomID: roomID2,
		HostScore:      session.Score1,
		OpponentScore:  session.Score2,
	})
	room2.SetPKState(&models.PKState{
		IsActive:       true,
		SessionID:      sessionID,
		OpponentID:     room1.HostID,
		OpponentRoomID: roomID1,
		HostScore:      session.Score2,
		OpponentScore:  session.Score1,
	})
	pkm.roomManager.SyncRoomState(roomID1)
	pkm.roomManager.SyncRoomState(roomID2)

	// Persist PKSession in Redis
	if broker := pkm.roomManager.GetBroker(); broker != nil && broker.IsActive() {
		_ = broker.SavePKSession(context.TODO(), session)
	}

	// Broadcast 'pk_started' signaling event to participants in both rooms
	pkPayload, _ := json.Marshal(map[string]any{
		"event":        "pk_started",
		"session_id":   sessionID,
		"room_1":       roomID1,
		"room_2":       roomID2,
		"room_a_id":    roomID1,
		"room_b_id":    roomID2,
		"host_1":       room1.HostID,
		"host_2":       room2.HostID,
		"host_1_score": session.Score1,
		"host_2_score": session.Score2,
		"status":       "active",
		"created_at":   session.CreatedAt.Unix(),
	})

	startMsg1 := &models.SignalingMessage{
		Event:   "pk_started",
		RoomID:  roomID1,
		Payload: pkPayload,
	}
	startMsg2 := &models.SignalingMessage{
		Event:   "pk_started",
		RoomID:  roomID2,
		Payload: pkPayload,
	}

	_ = pkm.roomManager.BroadcastToRoom(roomID1, startMsg1)
	_ = pkm.roomManager.BroadcastToRoom(roomID2, startMsg2)

	log.Printf("PK battle successfully started between Room '%s' and Room '%s'.\n", roomID1, roomID2)
	return nil
}

// SyncPKScore calculates and broadcasts the updated score to both rooms and the combined PK channel
func (pkm *PKManager) SyncPKScore(roomID string, updatedScore int64) {
	session, exists := pkm.GetPKSession(roomID)
	if !exists || session == nil {
		return
	}

	// Fetch current scores
	var score1, score2 int64
	if broker := pkm.roomManager.GetBroker(); broker != nil && broker.IsActive() {
		score1, _ = broker.GetHostScore(context.TODO(), session.RoomID1)
		score2, _ = broker.GetHostScore(context.TODO(), session.RoomID2)
	}

	if r1, ok := pkm.roomManager.GetRoom(session.RoomID1); ok && r1 != nil {
		if score1 == 0 {
			score1 = int64(r1.GetHostScore())
		}
		r1.SetPKState(&models.PKState{
			IsActive:       true,
			SessionID:      session.SessionID,
			OpponentID:     session.HostID2,
			OpponentRoomID: session.RoomID2,
			HostScore:      score1,
			OpponentScore:  score2,
		})
	}
	if r2, ok := pkm.roomManager.GetRoom(session.RoomID2); ok && r2 != nil {
		if score2 == 0 {
			score2 = int64(r2.GetHostScore())
		}
		r2.SetPKState(&models.PKState{
			IsActive:       true,
			SessionID:      session.SessionID,
			OpponentID:     session.HostID1,
			OpponentRoomID: session.RoomID1,
			HostScore:      score2,
			OpponentScore:  score1,
		})
	}

	scorePayload, _ := json.Marshal(map[string]any{
		"event":        "pk_score_update",
		"session_id":   session.SessionID,
		"room_a_id":    session.RoomID1,
		"room_b_id":    session.RoomID2,
		"room_1":       session.RoomID1,
		"room_2":       session.RoomID2,
		"room_a_score": score1,
		"room_b_score": score2,
		"score_1":      score1,
		"score_2":      score2,
		"host_1_score": score1,
		"host_2_score": score2,
	})

	scoreMsg := &models.SignalingMessage{
		Event:   "pk_score_update",
		RoomID:  session.RoomID1,
		Payload: scorePayload,
	}

	_ = pkm.roomManager.BroadcastToRoom(session.RoomID1, scoreMsg)

	scoreMsg2 := &models.SignalingMessage{
		Event:   "pk_score_update",
		RoomID:  session.RoomID2,
		Payload: scorePayload,
	}
	_ = pkm.roomManager.BroadcastToRoom(session.RoomID2, scoreMsg2)

	if broker := pkm.roomManager.GetBroker(); broker != nil && broker.IsActive() {
		_ = broker.PublishPKEvent(session.SessionID, scoreMsg)
	}

	log.Printf("[PK Score Sync] Session %s -> Room A (%s): %d | Room B (%s): %d\n", session.SessionID, session.RoomID1, score1, session.RoomID2, score2)
}

// StopPK ends an active PK battle session, cleans up cross-room tracks, and notifies participants
func (pkm *PKManager) StopPK(roomID string) error {
	pkm.mu.Lock()
	session, exists := pkm.activePKs[roomID]
	if !exists {
		pkm.mu.Unlock()
		return errors.New("no active PK session found for room: " + roomID)
	}

	delete(pkm.activePKs, session.RoomID1)
	delete(pkm.activePKs, session.RoomID2)
	pkm.mu.Unlock()

	// Clean up from Redis
	if broker := pkm.roomManager.GetBroker(); broker != nil && broker.IsActive() {
		_ = broker.DeletePKSession(context.TODO(), session.RoomID1, session.RoomID2)
	}

	// Remove cross-routed tracks & clear PKState on both rooms
	if r1, ok := pkm.roomManager.GetRoom(session.RoomID1); ok && r1 != nil {
		r1.SetPKState(nil)
		pkm.roomManager.SyncRoomState(session.RoomID1)
		pkm.roomManager.RemoveTrackAndRenegotiate(session.RoomID2, "pk-"+r1.HostID)
	}
	if r2, ok := pkm.roomManager.GetRoom(session.RoomID2); ok && r2 != nil {
		r2.SetPKState(nil)
		pkm.roomManager.SyncRoomState(session.RoomID2)
		pkm.roomManager.RemoveTrackAndRenegotiate(session.RoomID1, "pk-"+r2.HostID)
	}

	endPayload, _ := json.Marshal(map[string]any{
		"event":      "pk_ended",
		"session_id": session.SessionID,
		"room_1":     session.RoomID1,
		"room_2":     session.RoomID2,
		"status":     "ended",
	})

	_ = pkm.roomManager.BroadcastToRoom(session.RoomID1, &models.SignalingMessage{
		Event:   "pk_ended",
		RoomID:  session.RoomID1,
		Payload: endPayload,
	})

	_ = pkm.roomManager.BroadcastToRoom(session.RoomID2, &models.SignalingMessage{
		Event:   "pk_ended",
		RoomID:  session.RoomID2,
		Payload: endPayload,
	})

	log.Printf("PK battle ended between Room '%s' and Room '%s'.\n", session.RoomID1, session.RoomID2)
	return nil
}

// GetPKSession retrieves the active PK session for a room
func (pkm *PKManager) GetPKSession(roomID string) (*models.PKSession, bool) {
	pkm.mu.RLock()
	session, exists := pkm.activePKs[roomID]
	pkm.mu.RUnlock()

	if exists && session != nil {
		return session, true
	}

	// Fallback to Redis
	if broker := pkm.roomManager.GetBroker(); broker != nil && broker.IsActive() {
		if redisSession, err := broker.GetPKSession(context.TODO(), roomID); err == nil && redisSession != nil {
			pkm.mu.Lock()
			pkm.activePKs[roomID] = redisSession
			pkm.mu.Unlock()
			return redisSession, true
		}
	}

	return nil, false
}
