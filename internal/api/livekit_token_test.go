package api

import (
	"testing"
	"time"

	"github.com/livekit/protocol/auth"
)

func TestGenerateLiveKitToken_HostPermissions(t *testing.T) {
	apiKey := "devkey"
	apiSecret := "secretkey123456789012345678901234"
	roomID := "room-alpha"
	userID := "host-alice"
	userName := "Alice Host"
	role := "host"

	tokenStr, err := GenerateLiveKitToken(apiKey, apiSecret, roomID, userID, userName, role, nil, nil, 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate host token: %v", err)
	}
	if tokenStr == "" {
		t.Fatal("Expected non-empty token string")
	}

	// Verify claims using LiveKit auth parser
	verifier, err := auth.ParseAPIToken(tokenStr)
	if err != nil {
		t.Fatalf("Failed to parse token: %v", err)
	}

	_, grants, err := verifier.Verify(apiSecret)
	if err != nil {
		t.Fatalf("Failed to verify token: %v", err)
	}

	if grants.Identity != userID {
		t.Errorf("Expected identity '%s', got '%s'", userID, grants.Identity)
	}
	if grants.Video == nil {
		t.Fatal("Expected video grant to be present")
	}
	if grants.Video.Room != roomID {
		t.Errorf("Expected room '%s', got '%s'", roomID, grants.Video.Room)
	}
	if !grants.Video.RoomJoin {
		t.Errorf("Expected RoomJoin to be true")
	}
	if !grants.Video.GetCanPublish() {
		t.Errorf("Expected Host CanPublish to be true")
	}
	if !grants.Video.GetCanSubscribe() {
		t.Errorf("Expected Host CanSubscribe to be true")
	}
	if !grants.Video.RoomAdmin {
		t.Errorf("Expected Host RoomAdmin to be true")
	}
}

func TestGenerateLiveKitToken_ViewerPermissions(t *testing.T) {
	apiKey := "devkey"
	apiSecret := "secretkey123456789012345678901234"
	roomID := "room-alpha"
	userID := "viewer-bob"
	userName := "Bob Viewer"
	role := "viewer"

	tokenStr, err := GenerateLiveKitToken(apiKey, apiSecret, roomID, userID, userName, role, nil, nil, 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate viewer token: %v", err)
	}

	verifier, err := auth.ParseAPIToken(tokenStr)
	if err != nil {
		t.Fatalf("Failed to parse token: %v", err)
	}

	_, grants, err := verifier.Verify(apiSecret)
	if err != nil {
		t.Fatalf("Failed to verify token: %v", err)
	}

	if grants.Video == nil {
		t.Fatal("Expected video grant to be present")
	}
	if !grants.Video.RoomJoin {
		t.Errorf("Expected Viewer RoomJoin to be true")
	}
	if grants.Video.GetCanPublish() {
		t.Errorf("Expected Viewer CanPublish to be false")
	}
	if !grants.Video.GetCanSubscribe() {
		t.Errorf("Expected Viewer CanSubscribe to be true")
	}
}
