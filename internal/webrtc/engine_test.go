package webrtc

import (
	"testing"
)

func TestInitWebRTC(t *testing.T) {
	api, err := InitWebRTC()
	if err != nil {
		t.Fatalf("InitWebRTC failed: %v", err)
	}
	if api == nil {
		t.Fatal("Expected non-nil webrtc.API instance")
	}
}
