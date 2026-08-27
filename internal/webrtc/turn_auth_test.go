package webrtc

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	pionWebRTC "github.com/pion/webrtc/v3"
)

func TestGenerateTURNCredentials(t *testing.T) {
	secret := "test_secret_turn_12345"
	userID := "user_alex"
	duration := 1 * time.Hour

	username, password := GenerateTURNCredentials(userID, secret, duration)
	if username == "" || password == "" {
		t.Fatalf("Expected non-empty username and password, got '%s', '%s'", username, password)
	}

	parts := strings.Split(username, ":")
	if len(parts) != 2 {
		t.Fatalf("Expected username format [timestamp]:[user_id], got %s", username)
	}

	expiryUnix, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		t.Fatalf("Failed to parse expiry timestamp: %v", err)
	}
	if expiryUnix <= time.Now().Unix() {
		t.Fatalf("Expected future expiry timestamp, got %d", expiryUnix)
	}
	if parts[1] != userID {
		t.Fatalf("Expected user_id '%s' in username, got '%s'", userID, parts[1])
	}

	// Verify HMAC-SHA1 signature
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(username))
	expectedPassword := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	if password != expectedPassword {
		t.Fatalf("Password mismatch! Expected '%s', got '%s'", expectedPassword, password)
	}
}

func TestGetDefaultICEServers(t *testing.T) {
	_ = os.Setenv("PUBLIC_IP", "127.0.0.1")
	_ = os.Setenv("TURN_SECRET", "test_turn_secret")

	servers := GetDefaultICEServers("usr_test")
	if len(servers) < 2 {
		t.Fatalf("Expected at least 2 ICE server configurations, got %d", len(servers))
	}

	// Check STUN
	stunServer := servers[0]
	if len(stunServer.URLs) == 0 || !strings.HasPrefix(stunServer.URLs[0], "stun:") {
		t.Fatalf("Expected STUN server first, got: %+v", stunServer)
	}

	// Check TURN
	turnServer := servers[1]
	if len(turnServer.URLs) == 0 || !strings.HasPrefix(turnServer.URLs[0], "turn:") {
		t.Fatalf("Expected TURN server second, got: %+v", turnServer)
	}
	if turnServer.Username == "" || turnServer.Credential != "" && turnServer.CredentialType != pionWebRTC.ICECredentialTypePassword {
		t.Fatalf("Expected password credential type for TURN, got: %+v", turnServer)
	}

	config := GetDynamicRTCConfiguration("usr_test")
	if len(config.ICEServers) != len(servers) {
		t.Fatalf("Expected dynamic RTCConfiguration to contain all servers")
	}
}
