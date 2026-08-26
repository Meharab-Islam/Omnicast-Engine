package signaling

import (
	"testing"
	"time"
)

func TestJWTAuthentication(t *testing.T) {
	secret := "test_jwt_secret_key_12345"

	// 1. Generate valid token
	token, err := GenerateToken("user-host-1", "host", "room-101", secret, 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// 2. Validate token
	claims, err := ValidateToken(token, secret)
	if err != nil {
		t.Fatalf("ValidateToken failed on valid token: %v", err)
	}

	if claims.UserID != "user-host-1" {
		t.Errorf("Expected user_id 'user-host-1', got '%s'", claims.UserID)
	}
	if claims.Role != "host" {
		t.Errorf("Expected role 'host', got '%s'", claims.Role)
	}
	if claims.RoomID != "room-101" {
		t.Errorf("Expected room_id 'room-101', got '%s'", claims.RoomID)
	}

	// 3. Test Invalid Secret
	_, err = ValidateToken(token, "wrong_secret_key")
	if err == nil {
		t.Error("Expected error validating token with wrong secret, got nil")
	}

	// 4. Test Expired Token
	expiredToken, err := GenerateToken("user-viewer-2", "viewer", "room-101", secret, -1*time.Minute)
	if err != nil {
		t.Fatalf("Failed to generate expired token: %v", err)
	}

	_, err = ValidateToken(expiredToken, secret)
	if err == nil {
		t.Error("Expected error validating expired token, got nil")
	}

	// 5. Test Empty Token
	_, err = ValidateToken("", secret)
	if err == nil {
		t.Error("Expected error validating empty token, got nil")
	}
}
