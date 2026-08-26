package signaling

import (
	"testing"
)

func TestPKManager(t *testing.T) {
	rm := NewRoomManager()
	pkm := NewPKManager(rm)

	// Create Room 1 and Room 2
	_, err := rm.CreateRoom("room-A", "host-A")
	if err != nil {
		t.Fatalf("Failed to create room-A: %v", err)
	}

	_, err = rm.CreateRoom("room-B", "host-B")
	if err != nil {
		t.Fatalf("Failed to create room-B: %v", err)
	}

	// Add viewers to room 1 and room 2
	viewerA := &Client{ID: "viewer-A", Send: make(chan []byte, 10)}
	viewerB := &Client{ID: "viewer-B", Send: make(chan []byte, 10)}

	_ = rm.JoinViewer("room-A", viewerA)
	_ = rm.JoinViewer("room-B", viewerB)

	// Test StartPK
	err = pkm.StartPK("room-A", "room-B")
	if err != nil {
		t.Fatalf("StartPK failed: %v", err)
	}

	// Verify session exists
	session, exists := pkm.GetPKSession("room-A")
	if !exists || session == nil {
		t.Fatal("Expected active PK session for room-A")
	}

	// Test StopPK
	err = pkm.StopPK("room-A")
	if err != nil {
		t.Fatalf("StopPK failed: %v", err)
	}

	_, exists = pkm.GetPKSession("room-A")
	if exists {
		t.Error("Expected PK session for room-A to be removed")
	}
}
