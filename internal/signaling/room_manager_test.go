package signaling

import (
	"encoding/json"
	"testing"

	"live-media-server/internal/models"
)

func TestRoomManager(t *testing.T) {
	rm := NewRoomManager()

	// Test CreateRoom
	room, err := rm.CreateRoom("room-101", "host-user-1")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	if room.RoomID != "room-101" {
		t.Errorf("Expected RoomID 'room-101', got '%s'", room.RoomID)
	}

	hostClient := &Client{ID: "host-user-1", Send: make(chan []byte, 10)}
	room.SetHostClient(hostClient)

	// Test AddHostScore
	newScore := room.AddHostScore(50)
	if newScore != 50 || room.GetHostScore() != 50 {
		t.Errorf("Expected host score 50, got %d", newScore)
	}
	newScore = room.AddHostScore(50)
	if newScore != 100 || room.GetHostScore() != 100 {
		t.Errorf("Expected host score 100, got %d", newScore)
	}

	// Test Duplicate Room
	_, err = rm.CreateRoom("room-101", "host-user-2")
	if err == nil {
		t.Error("Expected error on duplicate room creation, got nil")
	}

	// Test GetRoom
	fetchedRoom, exists := rm.GetRoom("room-101")
	if !exists || fetchedRoom == nil {
		t.Fatal("Expected room-101 to exist")
	}

	// Test JoinViewer
	dummyClient := &Client{ID: "viewer-1", Send: make(chan []byte, 10)}
	err = rm.JoinViewer("room-101", dummyClient)
	if err != nil {
		t.Fatalf("JoinViewer failed: %v", err)
	}
	if fetchedRoom.ViewersCount() != 1 {
		t.Errorf("Expected 1 viewer, got %d", fetchedRoom.ViewersCount())
	}

	// Drain viewer_count broadcast messages sent upon join
	for len(hostClient.Send) > 0 {
		<-hostClient.Send
	}
	for len(dummyClient.Send) > 0 {
		<-dummyClient.Send
	}

	// Test Chat Broadcast to Host and Viewers
	chatMsg := &models.SignalingMessage{
		Event:   "chat",
		RoomID:  "room-101",
		UserID:  "viewer-1",
		Payload: json.RawMessage(`{"text":"hello everyone"}`),
	}
	err = rm.BroadcastToRoom("room-101", chatMsg)
	if err != nil {
		t.Fatalf("BroadcastToRoom failed: %v", err)
	}

	// Verify Host received chat
	select {
	case hostBytes := <-hostClient.Send:
		var parsedHost models.SignalingMessage
		_ = json.Unmarshal(hostBytes, &parsedHost)
		if parsedHost.Event != "chat" {
			t.Errorf("Expected host to receive 'chat', got %s", parsedHost.Event)
		}
	default:
		t.Error("Expected host to receive broadcasted chat")
	}

	// Verify Viewer received chat
	select {
	case viewerBytes := <-dummyClient.Send:
		var parsedViewer models.SignalingMessage
		_ = json.Unmarshal(viewerBytes, &parsedViewer)
		if parsedViewer.Event != "chat" {
			t.Errorf("Expected viewer to receive 'chat', got %s", parsedViewer.Event)
		}
	default:
		t.Error("Expected viewer to receive broadcasted chat")
	}

	// Test Viewer Disconnect
	rm.HandleClientDisconnect(dummyClient)
	if fetchedRoom.ViewersCount() != 0 {
		t.Errorf("Expected 0 viewers after viewer disconnect, got %d", fetchedRoom.ViewersCount())
	}

	// Drain messages
	for len(hostClient.Send) > 0 {
		<-hostClient.Send
	}

	// Re-add viewer and test Host Disconnect
	viewer2 := &Client{ID: "viewer-2", Send: make(chan []byte, 10)}
	_ = rm.JoinViewer("room-101", viewer2)

	// Drain viewer_count from viewer2
	for len(viewer2.Send) > 0 {
		<-viewer2.Send
	}

	rm.HandleClientDisconnect(hostClient)

	// Verify room is in Grace Period reconnection state
	if !fetchedRoom.IsReconnecting() {
		t.Error("Expected room to enter grace period reconnection state")
	}

	// Check if viewer received 'host_reconnecting'
	select {
	case msgBytes := <-viewer2.Send:
		var msg models.SignalingMessage
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			t.Fatalf("Failed to parse host_reconnecting message: %v", err)
		}
		if msg.Event != "host_reconnecting" {
			t.Errorf("Expected event 'host_reconnecting', got '%s'", msg.Event)
		}
	default:
		t.Error("Expected viewer to receive 'host_reconnecting' message on host disconnect")
	}

	// Explicitly close room to simulate grace period expiration
	rm.CloseRoomAndNotify("room-101", hostClient.ID)

	// Check if room was deleted
	if rm.ActiveRoomsCount() != 0 {
		t.Errorf("Expected 0 active rooms after host grace period close, got %d", rm.ActiveRoomsCount())
	}

	// Check if viewer received 'room_closed'
	select {
	case msgBytes := <-viewer2.Send:
		var msg models.SignalingMessage
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			t.Fatalf("Failed to parse room_closed message: %v", err)
		}
		if msg.Event != "room_closed" {
			t.Errorf("Expected event 'room_closed', got '%s'", msg.Event)
		}
	default:
		t.Error("Expected viewer to receive 'room_closed' message on close")
	}
}

func TestGetAllRooms(t *testing.T) {
	rm := NewRoomManager()

	// 1. Create Room A and Room B
	roomA, err := rm.CreateRoom("room-A", "host-A")
	if err != nil {
		t.Fatalf("Failed to create room-A: %v", err)
	}
	roomA.SetRoomName("Gaming Stream VIP")

	roomB, err := rm.CreateRoom("room-B", "host-B")
	if err != nil {
		t.Fatalf("Failed to create room-B: %v", err)
	}
	roomB.SetRoomName("Music Live Concert")

	// 2. Add viewers
	viewer1 := &Client{ID: "viewer-1", Send: make(chan []byte, 10)}
	viewer2 := &Client{ID: "viewer-2", Send: make(chan []byte, 10)}
	_ = rm.JoinViewer("room-A", viewer1)
	_ = rm.JoinViewer("room-A", viewer2)

	// 3. Test GetAllRooms
	rooms := rm.GetAllRooms()
	if len(rooms) != 2 {
		t.Fatalf("Expected 2 active rooms, got %d", len(rooms))
	}

	foundA := false
	for _, r := range rooms {
		if r.RoomID == "room-A" {
			foundA = true
			if r.RoomName != "Gaming Stream VIP" {
				t.Errorf("Expected room_name 'Gaming Stream VIP', got '%s'", r.RoomName)
			}
			if r.HostID != "host-A" {
				t.Errorf("Expected host_id 'host-A', got '%s'", r.HostID)
			}
			if r.ViewerCount != 2 {
				t.Errorf("Expected viewer_count 2, got %d", r.ViewerCount)
			}
		}
	}

	if !foundA {
		t.Error("room-A not found in GetAllRooms result")
	}
}

func TestAddTrackAndRenegotiate(t *testing.T) {
	rm := NewRoomManager()
	_, err := rm.CreateRoom("room-multi", "main-host")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	viewer := &Client{ID: "viewer-guest", Send: make(chan []byte, 10)}
	_ = rm.JoinViewer("room-multi", viewer)

	// Call AddTrackAndRenegotiate with nil track (guard check)
	rm.AddTrackAndRenegotiate("room-multi", nil, "cohost-1")

	// Call AddTrackAndRenegotiate on non-existing room
	rm.AddTrackAndRenegotiate("non-existent-room", nil, "cohost-1")
}

func TestViewerUpdateEvent(t *testing.T) {
	rm := NewRoomManager()
	room, err := rm.CreateRoom("room-v-sync", "host-main")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	hostClient := &Client{ID: "host-main", Send: make(chan []byte, 10)}
	room.SetHostClient(hostClient)

	viewer1 := &Client{ID: "viewer-u1", Send: make(chan []byte, 10)}
	_ = rm.JoinViewer("room-v-sync", viewer1)

	// Check host received viewer_update
	select {
	case msgBytes := <-hostClient.Send:
		var msg models.SignalingMessage
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			t.Fatalf("Failed to parse viewer_update message: %v", err)
		}
		if msg.Event != "viewer_update" {
			t.Errorf("Expected event 'viewer_update', got '%s'", msg.Event)
		}
		if msg.TotalViewers != 1 {
			t.Errorf("Expected TotalViewers 1, got %d", msg.TotalViewers)
		}
		if len(msg.ViewersList) != 1 || msg.ViewersList[0] != "viewer-u1" {
			t.Errorf("Expected ViewersList ['viewer-u1'], got %v", msg.ViewersList)
		}
	default:
		t.Error("Expected host to receive viewer_update message")
	}

	// Remove viewer and check broadcast
	for len(hostClient.Send) > 0 {
		<-hostClient.Send
	}
	rm.RemoveViewer("room-v-sync", "viewer-u1")

	select {
	case msgBytes := <-hostClient.Send:
		var msg models.SignalingMessage
		_ = json.Unmarshal(msgBytes, &msg)
		if msg.Event != "viewer_update" {
			t.Errorf("Expected event 'viewer_update', got '%s'", msg.Event)
		}
		if msg.TotalViewers != 0 {
			t.Errorf("Expected TotalViewers 0 after removal, got %d", msg.TotalViewers)
		}
	default:
		t.Error("Expected host to receive viewer_update on removal")
	}
}
