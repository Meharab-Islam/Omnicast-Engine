package signaling

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pion/webrtc/v3"
	"omnicast/internal/models"
)

func TestICERestart_SeamlessRenegotiation(t *testing.T) {
	rm := NewRoomManager()
	hub := NewHub(rm)
	go hub.Run()

	room, err := rm.CreateRoom("ice-restart-room", "host-alice")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	mediaEngine := &webrtc.MediaEngine{}
	_ = mediaEngine.RegisterDefaultCodecs()
	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))

	// Server PC (Viewer Handler)
	serverPC, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create server PeerConnection: %v", err)
	}
	defer serverPC.Close()

	// Client Remote PC (Simulated Mobile App)
	clientPC, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create client PeerConnection: %v", err)
	}
	defer clientPC.Close()

	// Add dummy transceiver so offer has m-lines
	_, err = serverPC.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	if err != nil {
		t.Fatalf("Failed to add transceiver: %v", err)
	}

	client := &Client{
		ID:             "viewer-ice-test",
		RoomID:         room.RoomID,
		Role:           "viewer",
		Hub:            hub,
		RoomManager:    rm,
		PeerConnection: serverPC,
		Send:           make(chan []byte, 10),
	}

	_ = rm.JoinViewer(room.RoomID, client)

	// Flush any initial presence message from client.Send channel
	select {
	case <-client.Send:
	default:
	}

	// 1. Initial Offer / Answer to establish Stable signaling state
	initialOffer, err := serverPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("Failed to create initial offer: %v", err)
	}
	if err := serverPC.SetLocalDescription(initialOffer); err != nil {
		t.Fatalf("Failed to set local desc: %v", err)
	}
	if err := clientPC.SetRemoteDescription(initialOffer); err != nil {
		t.Fatalf("Failed to set remote desc: %v", err)
	}

	answer, err := clientPC.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("Failed to create answer: %v", err)
	}
	if err := clientPC.SetLocalDescription(answer); err != nil {
		t.Fatalf("Failed to set client local desc: %v", err)
	}
	if err := serverPC.SetRemoteDescription(answer); err != nil {
		t.Fatalf("Failed to set server remote desc: %v", err)
	}

	if serverPC.SignalingState() != webrtc.SignalingStateStable {
		t.Fatalf("Expected signaling state to be stable, got %s", serverPC.SignalingState())
	}

	// 2. Trigger ICE Restart (Simulating WiFi to Cellular network switch)
	client.TriggerICERestart(room.RoomID)

	var restartOffer webrtc.SessionDescription
	select {
	case msgBytes := <-client.Send:
		sigMsg, err := models.ParseSignalingMessage(msgBytes)
		if err != nil {
			t.Fatalf("Failed to parse restart signaling message: %v", err)
		}
		if sigMsg.Event != "offer" {
			t.Errorf("Expected event 'offer', got '%s'", sigMsg.Event)
		}

		if err := json.Unmarshal(sigMsg.Payload, &restartOffer); err != nil {
			t.Fatalf("Failed to unmarshal offer payload: %v", err)
		}
		if restartOffer.SDP == "" {
			t.Errorf("Expected non-empty SDP offer")
		}

		t.Logf("Successfully generated ICE restart offer for client %s", client.ID)

	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for ICE restart offer")
	}

	// Complete renegotiation with client answer to return serverPC to Stable
	if err := clientPC.SetRemoteDescription(restartOffer); err != nil {
		t.Fatalf("Failed to set restart offer on client: %v", err)
	}
	restartAnswer, err := clientPC.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("Failed to create restart answer: %v", err)
	}
	if err := clientPC.SetLocalDescription(restartAnswer); err != nil {
		t.Fatalf("Failed to set local desc for restart answer: %v", err)
	}
	if err := serverPC.SetRemoteDescription(restartAnswer); err != nil {
		t.Fatalf("Failed to apply restart answer on server: %v", err)
	}

	// 3. Test explicit handleICERestartRequest from client SDK
	client.handleICERestartRequest(&models.SignalingMessage{
		Event:  "ice_restart",
		RoomID: room.RoomID,
		UserID: client.ID,
	})

	select {
	case msgBytes := <-client.Send:
		sigMsg, _ := models.ParseSignalingMessage(msgBytes)
		if sigMsg.Event != "offer" {
			t.Errorf("Expected event 'offer' on explicit request, got '%s'", sigMsg.Event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for explicit ICE restart offer")
	}
}
