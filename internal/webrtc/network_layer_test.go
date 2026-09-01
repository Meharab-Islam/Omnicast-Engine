package webrtc

import (
	"fmt"
	"testing"
	"time"
)

func TestInitOmnicastNetworkLayer(t *testing.T) {
	cfg := NetworkConfig{
		Port:         34795, // Ephemeral test port
		PublicIP:     "127.0.0.1",
		Realm:        "test.omnicast.live",
		AuthSecret:   "secret_key_testing_123",
		RelayMinPort: 50300,
		RelayMaxPort: 50320,
	}

	stack, err := InitOmnicastNetworkLayer(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize Omnicast network layer: %v", err)
	}
	defer func() {
		_ = stack.Close()
	}()

	if stack.UDPMux == nil {
		t.Fatal("Expected UDPMux to be initialized")
	}

	// Test Token Validation
	expiry := time.Now().Add(2 * time.Hour).Unix()
	validToken := fmt.Sprintf("%d:user_bob_456", expiry)

	key, ok := ValidateHMACToken(validToken, cfg.Realm, cfg.AuthSecret)
	if !ok || len(key) == 0 {
		t.Fatalf("Expected valid HMAC token authentication")
	}

	// Test Expired Token
	expiredToken := fmt.Sprintf("%d:user_bob_456", time.Now().Add(-1*time.Hour).Unix())
	_, ok = ValidateHMACToken(expiredToken, cfg.Realm, cfg.AuthSecret)
	if ok {
		t.Fatalf("Expected expired token to be rejected")
	}
}
