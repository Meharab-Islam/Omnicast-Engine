package signaling

import (
	"encoding/json"
	"testing"

	"omnicast/internal/models"
)

func TestRoomManager_ForceEndRoomKillSwitch(t *testing.T) {
	rm := NewRoomManager()
	room, err := rm.CreateRoom("room-kill-101", "host-alice")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	viewer := &Client{ID: "viewer-bob", Role: "viewer", RoomID: "room-kill-101", Send: make(chan []byte, 10)}
	_ = rm.JoinViewer("room-kill-101", viewer)

	// Drain join messages
	for len(viewer.Send) > 0 {
		<-viewer.Send
	}

	// Trigger Kill Switch
	rm.ForceEndRoom("room-kill-101", "host-alice", "closed_by_host")

	// Verify room is removed from active map
	if _, exists := rm.GetRoom("room-kill-101"); exists {
		t.Fatalf("Expected room to be completely wiped from active rooms")
	}

	// Verify viewer received room_ended message
	select {
	case msgBytes := <-viewer.Send:
		var parsed models.SignalingMessage
		_ = json.Unmarshal(msgBytes, &parsed)
		if parsed.Event != "room_ended" {
			t.Fatalf("Expected 'room_ended' event, got: %s", parsed.Event)
		}
	default:
		t.Fatalf("Expected viewer to receive 'room_ended' broadcast")
	}

	_ = room
}
