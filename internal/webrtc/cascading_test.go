package webrtc

import (
	"testing"

	"github.com/pion/webrtc/v3"
	"live-media-server/internal/models"
)

func TestCascadeManager_Initialization(t *testing.T) {
	api, err := InitWebRTC()
	if err != nil {
		t.Fatalf("InitWebRTC failed: %v", err)
	}

	cm := NewCascadeManager(api, nil, "test-edge-1")
	if cm == nil {
		t.Fatal("expected non-nil CascadeManager")
	}

	// Test EnsureCascaded on nil room
	err = cm.EnsureCascaded(nil)
	if err == nil {
		t.Fatal("expected error when EnsureCascaded on nil room")
	}

	// Test EnsureCascaded on room with active track (should no-op and return nil)
	room := models.NewRoom("room-cascaded", "host-user")
	track, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "video", "stream")
	if err != nil {
		t.Fatalf("failed to create track: %v", err)
	}
	room.SetVideoTrack(track)

	err = cm.EnsureCascaded(room)
	if err != nil {
		t.Fatalf("expected nil error when room already has active track, got: %v", err)
	}

	// Test Close
	cm.CloseSession("room-cascaded")
	cm.Close()
}
