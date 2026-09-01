package webrtc

import (
	"testing"

	"github.com/pion/webrtc/v3"
)

func TestInitOmnicastMediaEngine(t *testing.T) {
	mediaEngine, interceptors, err := InitOmnicastMediaEngine()
	if err != nil {
		t.Fatalf("Failed to initialize Omnicast MediaEngine: %v", err)
	}
	if mediaEngine == nil {
		t.Fatal("Expected mediaEngine to be non-nil")
	}
	if interceptors == nil {
		t.Fatal("Expected interceptorRegistry to be non-nil")
	}

	// 1. Create WebRTC API and verify PeerConnection with VP8/Opus
	settingEngine := webrtc.SettingEngine{}
	settingEngine.SetReceiveMTU(1500)

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(interceptors),
		webrtc.WithSettingEngine(settingEngine),
	)

	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create PeerConnection: %v", err)
	}
	defer func() {
		_ = pc.Close()
	}()

	// 2. Add VP8 video track
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000},
		"video",
		"stream",
	)
	if err != nil {
		t.Fatalf("Failed to create VP8 track: %v", err)
	}
	_, err = pc.AddTrack(videoTrack)
	if err != nil {
		t.Fatalf("Failed to add VP8 track to PeerConnection: %v", err)
	}

	// 3. Add Opus audio track
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
		"audio",
		"stream",
	)
	if err != nil {
		t.Fatalf("Failed to create Opus track: %v", err)
	}
	_, err = pc.AddTrack(audioTrack)
	if err != nil {
		t.Fatalf("Failed to add Opus track to PeerConnection: %v", err)
	}
}

func TestNewOmnicastWebRTCAPI(t *testing.T) {
	settingEngine := webrtc.SettingEngine{}
	settingEngine.SetReceiveMTU(1500)

	api, err := NewOmnicastWebRTCAPI(settingEngine)
	if err != nil {
		t.Fatalf("Failed to create Omnicast WebRTC API: %v", err)
	}
	if api == nil {
		t.Fatal("Expected non-nil WebRTC API")
	}

	// Create test PeerConnection with the generated API
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create PeerConnection with Omnicast API: %v", err)
	}
	defer func() {
		_ = pc.Close()
	}()
}
