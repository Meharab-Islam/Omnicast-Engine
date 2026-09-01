package webrtc

import (
	"fmt"
	"testing"
	"time"
)

func TestValidateAndGenerateAuthKey(t *testing.T) {
	secret := "test_secret_123"
	realm := "omnicast.live"
	userID := "mobile_client_99"

	// 1. Valid non-expired username
	expiry := time.Now().Add(1 * time.Hour).Unix()
	validUsername := fmt.Sprintf("%d:%s", expiry, userID)

	key, ok := ValidateAndGenerateAuthKey(validUsername, realm, secret)
	if !ok || len(key) == 0 {
		t.Fatalf("Expected valid auth key for unexpired token")
	}

	// 2. Expired username
	expiredUnix := time.Now().Add(-1 * time.Hour).Unix()
	expiredUsername := fmt.Sprintf("%d:%s", expiredUnix, userID)

	_, ok = ValidateAndGenerateAuthKey(expiredUsername, realm, secret)
	if ok {
		t.Fatalf("Expected expired token to be rejected")
	}

	// 3. Malformed username
	_, ok = ValidateAndGenerateAuthKey("invalid_format", realm, secret)
	if ok {
		t.Fatalf("Expected malformed username to be rejected")
	}
}

func TestStartEmbeddedTURNServer(t *testing.T) {
	cfg := EmbeddedTURNServerConfig{
		PublicIP: "127.0.0.1",
		Realm:    "test.realm",
		Secret:   "test_turn_secret",
		Port:     34789, // Use ephemeral test port
		MinPort:  50100,
		MaxPort:  50120,
	}

	server, err := StartEmbeddedTURNServer(cfg)
	if err != nil {
		t.Fatalf("Failed to start embedded TURN server: %v", err)
	}
	defer func() {
		_ = server.Close()
	}()
}
