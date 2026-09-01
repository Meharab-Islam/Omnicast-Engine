package signaling

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestVerifyWebSocketToken_Host(t *testing.T) {
	jwtSecret := "test_jwt_secret_key_123"
	dm := NewDistributedRoomManager(nil, "node-A", "127.0.0.1", jwtSecret)

	// Create a valid Host token
	claims := OmnicastAuthClaims{
		UserID:       "alice_host",
		RoomID:       "room_alpha",
		Role:         "host",
		CanPublish:   true,
		CanSubscribe: true,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		t.Fatalf("Failed to sign token: %v", err)
	}

	parsedClaims, err := dm.VerifyWebSocketToken(tokenStr)
	if err != nil {
		t.Fatalf("Failed to verify valid host token: %v", err)
	}
	if parsedClaims.UserID != "alice_host" {
		t.Errorf("Expected user_id 'alice_host', got '%s'", parsedClaims.UserID)
	}
	if !parsedClaims.CanPublish {
		t.Errorf("Expected CanPublish to be true for host")
	}

	// Verify CheckPermission
	if err := dm.CheckPermission(parsedClaims, "publish"); err != nil {
		t.Errorf("Expected Host to have publish permission: %v", err)
	}
}

func TestVerifyWebSocketToken_Viewer(t *testing.T) {
	jwtSecret := "test_jwt_secret_key_123"
	dm := NewDistributedRoomManager(nil, "node-B", "127.0.0.1", jwtSecret)

	// Create a valid Viewer token (CanPublish: false)
	claims := OmnicastAuthClaims{
		UserID:       "bob_viewer",
		RoomID:       "room_alpha",
		Role:         "viewer",
		CanPublish:   false,
		CanSubscribe: true,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		t.Fatalf("Failed to sign token: %v", err)
	}

	parsedClaims, err := dm.VerifyWebSocketToken(tokenStr)
	if err != nil {
		t.Fatalf("Failed to verify valid viewer token: %v", err)
	}

	// Verify Viewer can subscribe but CANNOT publish
	if err := dm.CheckPermission(parsedClaims, "subscribe"); err != nil {
		t.Errorf("Expected Viewer to have subscribe permission: %v", err)
	}
	if err := dm.CheckPermission(parsedClaims, "publish"); err == nil {
		t.Fatalf("Expected Viewer publish permission to be REJECTED")
	}
}
