package signaling

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pion/webrtc/v3"
	"omnicast/internal/models"
)

func TestNativePKBridging_CrossTrackAndTargetedGifting(t *testing.T) {
	rm := NewRoomManager()
	hub := NewHub(rm)
	go hub.Run()

	pkm := NewPKManager(rm)
	rm.SetPKManager(pkm)

	// Create Room A and Room B
	roomA, err := rm.CreateRoom("room-A", "host-A")
	if err != nil {
		t.Fatalf("Failed to create room A: %v", err)
	}
	roomB, err := rm.CreateRoom("room-B", "host-B")
	if err != nil {
		t.Fatalf("Failed to create room B: %v", err)
	}

	// Create dummy media tracks for hosts
	videoTrackA, _ := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video-A", "stream-A")
	audioTrackA, _ := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio-A", "stream-A")
	roomA.VideoTrack = videoTrackA
	roomA.AudioTrack = audioTrackA

	videoTrackB, _ := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video-B", "stream-B")
	audioTrackB, _ := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio-B", "stream-B")
	roomB.VideoTrack = videoTrackB
	roomB.AudioTrack = audioTrackB

	// Create Viewer in Room A and Viewer in Room B
	viewerA := &Client{
		ID:          "viewer-alice",
		UserName:    "Alice",
		RoomID:      "room-A",
		Role:        "viewer",
		Hub:         hub,
		RoomManager: rm,
		Send:        make(chan []byte, 50),
	}
	_ = rm.JoinViewer("room-A", viewerA)

	viewerB := &Client{
		ID:          "viewer-bob",
		UserName:    "Bob",
		RoomID:      "room-B",
		Role:        "viewer",
		Hub:         hub,
		RoomManager: rm,
		Send:        make(chan []byte, 50),
	}
	_ = rm.JoinViewer("room-B", viewerB)

	// Flush initial presence notifications
	time.Sleep(50 * time.Millisecond)
	for len(viewerA.Send) > 0 {
		<-viewerA.Send
	}
	for len(viewerB.Send) > 0 {
		<-viewerB.Send
	}

	// 1. LinkRooms(RoomA, RoomB)
	err = pkm.LinkRooms("room-A", "room-B")
	if err != nil {
		t.Fatalf("LinkRooms failed: %v", err)
	}

	// Check linked room state
	if roomA.GetLinkedRoom() != "room-B" {
		t.Errorf("Expected RoomA linkedRoomID to be room-B, got %s", roomA.GetLinkedRoom())
	}
	if roomB.GetLinkedRoom() != "room-A" {
		t.Errorf("Expected RoomB linkedRoomID to be room-A, got %s", roomB.GetLinkedRoom())
	}

	// Verify both rooms received pk_started
	select {
	case msgBytes := <-viewerA.Send:
		sigMsg, _ := models.ParseSignalingMessage(msgBytes)
		if sigMsg.Event != "pk_started" {
			t.Errorf("Expected pk_started in Room A, got %s", sigMsg.Event)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for pk_started in Room A")
	}

	select {
	case msgBytes := <-viewerB.Send:
		sigMsg, _ := models.ParseSignalingMessage(msgBytes)
		if sigMsg.Event != "pk_started" {
			t.Errorf("Expected pk_started in Room B, got %s", sigMsg.Event)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for pk_started in Room B")
	}

	// 2. Cross-Room Chat Mesh Broadcast
	chatPayload, _ := json.Marshal(map[string]any{"text": "Hello from Room A!"})
	viewerA.handleChatMessage(&models.SignalingMessage{
		Event:   "chat",
		RoomID:  "room-A",
		UserID:  viewerA.ID,
		Payload: chatPayload,
	})

	// Verify chat received in Room A and cross-room in Room B
	select {
	case msgBytes := <-viewerA.Send:
		sigMsg, _ := models.ParseSignalingMessage(msgBytes)
		if sigMsg.Event != "chat" {
			t.Errorf("Expected chat in Room A, got %s", sigMsg.Event)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for chat in Room A")
	}

	select {
	case msgBytes := <-viewerB.Send:
		sigMsg, _ := models.ParseSignalingMessage(msgBytes)
		if sigMsg.Event != "chat" {
			t.Errorf("Expected cross-room chat in Room B, got %s", sigMsg.Event)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for cross-room chat in Room B")
	}

	// 3. Targeted Gifting API: Viewer in Room A sends gift targeted to Host B
	giftJSON, _ := json.Marshal(map[string]any{
		"gift_id":        "rose",
		"gift":           "rose",
		"target_host_id": "host-B",
		"receiver_id":    "host-B",
		"coins":          500,
	})

	viewerA.handleGiftMessage(&models.SignalingMessage{
		Action:  "gift",
		Event:   "gift",
		RoomID:  "room-A",
		UserID:  viewerA.ID,
		Payload: giftJSON,
	})

	// Verify Host B received points
	if roomB.GetHostScore() != 500 {
		t.Errorf("Expected Host B score to be 500, got %d", roomB.GetHostScore())
	}
	if roomA.GetHostScore() != 0 {
		t.Errorf("Expected Host A score to remain 0, got %d", roomA.GetHostScore())
	}

	// Verify pk_gift_overlay received in BOTH Room A and Room B
	foundOverlayA := false
loopA:
	for i := 0; i < 3; i++ {
		select {
		case msgBytes := <-viewerA.Send:
			sigMsg, _ := models.ParseSignalingMessage(msgBytes)
			if sigMsg.Event == "pk_gift_overlay" || sigMsg.Action == "pk_gift_overlay" {
				foundOverlayA = true
				var overlay struct {
					Type        string `json:"type"`
					SenderName  string `json:"sender_name"`
					Gift        string `json:"gift"`
					ReceiverID  string `json:"receiver_id"`
					HostAPoints int64  `json:"host_a_points"`
					HostBPoints int64  `json:"host_b_points"`
				}
				_ = json.Unmarshal(sigMsg.Payload, &overlay)
				if overlay.ReceiverID != "host-B" {
					t.Errorf("Expected receiver_id host-B, got %s", overlay.ReceiverID)
				}
				if overlay.HostBPoints != 500 {
					t.Errorf("Expected host_b_points 500, got %d", overlay.HostBPoints)
				}
				break loopA
			}
		case <-time.After(1 * time.Second):
		}
	}
	if !foundOverlayA {
		t.Fatal("Failed to receive pk_gift_overlay in Room A")
	}

	foundOverlayB := false
loopB:
	for i := 0; i < 3; i++ {
		select {
		case msgBytes := <-viewerB.Send:
			sigMsg, _ := models.ParseSignalingMessage(msgBytes)
			if sigMsg.Event == "pk_gift_overlay" || sigMsg.Action == "pk_gift_overlay" {
				foundOverlayB = true
				break loopB
			}
		case <-time.After(1 * time.Second):
		}
	}
	if !foundOverlayB {
		t.Fatal("Failed to receive pk_gift_overlay in Room B")
	}

	// 4. UnlinkRooms(RoomA)
	err = pkm.UnlinkRooms("room-A")
	if err != nil {
		t.Fatalf("UnlinkRooms failed: %v", err)
	}

	if roomA.GetLinkedRoom() != "" {
		t.Errorf("Expected RoomA linkedRoomID to be cleared, got %s", roomA.GetLinkedRoom())
	}
}
