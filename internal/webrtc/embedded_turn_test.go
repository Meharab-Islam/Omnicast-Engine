package webrtc

import (
	"testing"
	"time"
)

func TestEmbeddedTURNServer_InitializationAndCredentials(t *testing.T) {
	cfg := EmbeddedTURNConfig{
		PublicIP:   "127.0.0.1",
		Port:       34789, // test port
		Realm:      "test.omnicast.live",
		AuthSecret: "super_secret_turn_key",
	}

	turnServer, err := NewEmbeddedTURNServer(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize EmbeddedTURNServer: %v", err)
	}
	defer turnServer.Close()

	// Test credential generation
	u, p := turnServer.GenerateEphemeralCredentials("test_user_1", 1*time.Hour)
	if u == "" || p == "" {
		t.Fatalf("expected valid username and password, got '%s' / '%s'", u, p)
	}

	if !ValidateEphemeralCredential(u) {
		t.Fatalf("expected newly generated credential to be valid")
	}

	// Test expired credential validation
	if ValidateEphemeralCredential("1000:expired_user") {
		t.Fatalf("expected past unix timestamp to be invalid")
	}
}
