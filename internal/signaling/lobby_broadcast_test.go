package signaling

import (
	"encoding/json"
	"testing"
	"time"

	"omnicast/internal/models"
)

func TestLobbyRealTimeUpdates(t *testing.T) {
	rm := NewRoomManager()
	hub := NewHub(rm)
	go hub.Run()

	// 1. Create global_lobby room and connect a lobby client
	lobbyRoom, err := rm.CreateRoom("global_lobby", "system")
	if err != nil {
		t.Fatalf("Failed to create global_lobby: %v", err)
	}

	lobbyClient := &Client{
		ID:          "lobby-user-1",
		RoomID:      "global_lobby",
		Role:        "viewer",
		Hub:         hub,
		RoomManager: rm,
		Send:        make(chan []byte, 50),
	}
	_ = rm.JoinViewer("global_lobby", lobbyClient)

	// 2. Broadcaster creates a new live room
	liveRoom, err := rm.CreateRoom("room-live-99", "host-alex")
	if err != nil {
		t.Fatalf("Failed to create live room: %v", err)
	}

	// Verify lobby client received "room_created" event
	select {
	case rawMsg := <-lobbyClient.Send:
		var sigMsg models.SignalingMessage
		if err := json.Unmarshal(rawMsg, &sigMsg); err != nil {
			t.Fatalf("Failed to parse signaling message: %v", err)
		}
		if sigMsg.Event != "room_created" {
			t.Fatalf("Expected event 'room_created', got '%s'", sigMsg.Event)
		}
		var payload struct {
			Event  string `json:"event"`
			RoomID string `json:"room_id"`
		}
		_ = json.Unmarshal(sigMsg.Payload, &payload)
		if payload.RoomID != "room-live-99" {
			t.Fatalf("Expected room_id 'room-live-99', got '%s'", payload.RoomID)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for 'room_created' event in global_lobby")
	}

	// 3. Close the live room
	rm.CloseRoomAndNotifyWithReason(liveRoom.RoomID, "host-alex", "stream_ended")

	// Verify lobby client received "room_closed" event
	select {
	case rawMsg := <-lobbyClient.Send:
		var sigMsg models.SignalingMessage
		if err := json.Unmarshal(rawMsg, &sigMsg); err != nil {
			t.Fatalf("Failed to parse signaling message: %v", err)
		}
		if sigMsg.Event != "room_closed" {
			t.Fatalf("Expected event 'room_closed', got '%s'", sigMsg.Event)
		}
		var payload struct {
			Event  string `json:"event"`
			RoomID string `json:"room_id"`
		}
		_ = json.Unmarshal(sigMsg.Payload, &payload)
		if payload.RoomID != "room-live-99" {
			t.Fatalf("Expected room_id 'room-live-99', got '%s'", payload.RoomID)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for 'room_closed' event in global_lobby")
	}

	_ = lobbyRoom
}
