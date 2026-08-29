package signaling

import (
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v3"
)

func TestZombiePeerGC(t *testing.T) {
	rm := NewRoomManager()
	hub := NewHub(rm)
	go hub.Run()

	// Create Room and Host
	room, err := rm.CreateRoom("zombie-room", "host-alice")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Create PeerConnection for simulated zombie client
	api := webrtc.NewAPI()
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create peer connection: %v", err)
	}

	zombieClient := &Client{
		ID:             "zombie-peer-1",
		Hub:            hub,
		RoomManager:    rm,
		PeerConnection: pc,
		Send:           make(chan []byte, 10),
		closed:         false,
		mu:             sync.Mutex{},
	}

	// Set lastPongReceived to 20 seconds ago (older than 15s threshold)
	zombieClient.lastPongReceived.Store(time.Now().Unix() - 20)

	// Add to room viewers and hub
	_ = rm.JoinViewer(room.RoomID, zombieClient)
	hub.mu.Lock()
	hub.clients[zombieClient] = true
	hub.mu.Unlock()

	if hub.ClientsCount() != 1 {
		t.Fatalf("Expected 1 client in hub, got %d", hub.ClientsCount())
	}
	if room.ViewersCount() != 1 {
		t.Fatalf("Expected 1 viewer in room, got %d", room.ViewersCount())
	}

	// Run Hub GC with 15s timeout
	hub.cleanupZombiePeers(15 * time.Second)

	// Wait for unregister channel processing
	time.Sleep(100 * time.Millisecond)

	// Verify zombie peer is forcefully cleaned up
	if hub.ClientsCount() != 0 {
		t.Errorf("Expected 0 clients in hub after zombie cleanup, got %d", hub.ClientsCount())
	}
	if room.ViewersCount() != 0 {
		t.Errorf("Expected 0 viewers in room after zombie cleanup, got %d", room.ViewersCount())
	}
}
