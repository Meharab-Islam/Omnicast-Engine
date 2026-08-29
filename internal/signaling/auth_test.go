package signaling

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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
	if !claims.AllowsPublishing() {
		t.Errorf("Host should be allowed to publish by default")
	}
	if !claims.AllowsSubscribing() {
		t.Errorf("Host should be allowed to subscribe by default")
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

	// 6. Test GenerateUserToken with profile claims
	userToken, err := GenerateUserToken("u_bob", "Bob Smith", "https://img.com/bob.png", secret, 24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate user token: %v", err)
	}
	userClaims, err := ValidateToken(userToken, secret)
	if err != nil {
		t.Fatalf("Failed to validate user token: %v", err)
	}
	if userClaims.UserID != "u_bob" || userClaims.UserName != "Bob Smith" || userClaims.AvatarURL != "https://img.com/bob.png" {
		t.Fatalf("Unexpected user claims: %+v", userClaims)
	}

	// 7. Test Explicit Granular Permissions (can_publish: false, can_subscribe: true)
	canPublishFalse := false
	canSubscribeTrue := true
	customClaims := UserClaims{
		UserID:       "usr_restricted",
		Role:         "host",
		CanPublish:   &canPublishFalse,
		CanSubscribe: &canSubscribeTrue,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, customClaims)
	tokenStr, _ := tok.SignedString([]byte(secret))

	parsedClaims, err := ValidateToken(tokenStr, secret)
	if err != nil {
		t.Fatalf("Failed to validate token with custom claims: %v", err)
	}
	if parsedClaims.AllowsPublishing() {
		t.Errorf("Expected AllowsPublishing() to return false for can_publish: false")
	}
	if !parsedClaims.AllowsSubscribing() {
		t.Errorf("Expected AllowsSubscribing() to return true for can_subscribe: true")
	}
}
