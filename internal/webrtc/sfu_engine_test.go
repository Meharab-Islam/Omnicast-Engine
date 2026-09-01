package webrtc

import (
	"testing"
)

func TestNewSFUEngine(t *testing.T) {
	cfg := SFUEngineConfig{
		PublicIP:      "127.0.0.1",
		TURNPort:      34790, // Ephemeral test port
		TURNRealm:     "test.realm",
		TURNSecret:    "test_turn_secret",
		SingleUDPPort: 50190,
		RelayMinPort:  50200,
		RelayMaxPort:  50220,
	}

	engine, err := NewSFUEngine(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize SFUEngine: %v", err)
	}
	defer func() {
		_ = engine.Close()
	}()

	if engine.WebRTCAPI == nil {
		t.Fatal("Expected WebRTCAPI to be non-nil")
	}

	// Test creating a PeerConnection using the initialized WebRTC API
	pc, err := engine.WebRTCAPI.NewPeerConnection(GetDynamicRTCConfiguration("test_user_1"))
	if err != nil {
		t.Fatalf("Failed to create PeerConnection from SFUEngine: %v", err)
	}
	defer func() {
		_ = pc.Close()
	}()
}
