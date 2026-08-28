package webrtc

import (
	"testing"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

func TestTrackSwitcher_SequenceAndTimestampContinuity(t *testing.T) {
	outTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000},
		"video",
		"stream",
	)
	if err != nil {
		t.Fatalf("Failed to create outTrack: %v", err)
	}

	switcher := NewTrackSwitcher(outTrack, "q")
	if switcher.GetCurrentLayer() != "q" {
		t.Fatalf("Expected initial layer 'q', got '%s'", switcher.GetCurrentLayer())
	}

	// 1. Send packets on layer 'q'
	pkt1 := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 100,
			Timestamp:      1000,
			SSRC:           11111,
		},
		Payload: []byte{0x90, 0x80, 0x01},
	}
	_ = switcher.WriteRTP("q", pkt1)

	pkt2 := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 101,
			Timestamp:      4000,
			SSRC:           11111,
		},
		Payload: []byte{0x90, 0x80, 0x02},
	}
	_ = switcher.WriteRTP("q", pkt2)

	// 2. Request layer switch to 'h'
	switcher.SwitchLayer("h")
	if switcher.GetTargetLayer() != "h" {
		t.Fatalf("Expected target layer 'h', got '%s'", switcher.GetTargetLayer())
	}

	// Packets from 'q' while pending switch should still be processed until 'h' keyframe arrives
	pkt3 := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 102,
			Timestamp:      7000,
			SSRC:           11111,
		},
		Payload: []byte{0x10, 0x01, 0x03},
	}
	_ = switcher.WriteRTP("q", pkt3)

	// Send a non-keyframe (P-frame) on target layer 'h' -> should be dropped and NOT trigger layer switch
	pktHPframe := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 4999,
			Timestamp:      990000,
			SSRC:           22222,
		},
		Payload: []byte{0x10, 0x01, 0x00}, // VP8 Inter-frame (P-frame: bit0 = 1)
	}
	_ = switcher.WriteRTP("h", pktHPframe)

	if switcher.GetCurrentLayer() != "q" {
		t.Fatalf("Expected switcher to remain on 'q' when receiving P-frame on 'h', got '%s'", switcher.GetCurrentLayer())
	}

	// Another packet on old layer 'q' should still forward
	pkt4 := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 103,
			Timestamp:      10000,
			SSRC:           11111,
		},
		Payload: []byte{0x10, 0x01, 0x04},
	}
	_ = switcher.WriteRTP("q", pkt4)

	// 3. First Keyframe / I-frame packet from new layer 'h'
	pktH1 := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 5000,   // Different sequence space!
			Timestamp:      999999, // Different timestamp space!
			SSRC:           22222,
		},
		Payload: []byte{0x10, 0x00, 0x00}, // VP8 Keyframe (bit0 = 0)
	}
	_ = switcher.WriteRTP("h", pktH1)

	if switcher.GetCurrentLayer() != "h" {
		t.Fatalf("Expected current layer to switch to 'h', got '%s'", switcher.GetCurrentLayer())
	}

	// Check that last output sequence number is monotonic (should be 104, continuing from 103)
	if switcher.lastOutSeq != 104 {
		t.Fatalf("Expected rewritten lastOutSeq 104, got %d", switcher.lastOutSeq)
	}

	// 4. Subsequent packet on 'h'
	pktH2 := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 5001,
			Timestamp:      1003000,
			SSRC:           22222,
		},
		Payload: []byte{0x90, 0x80, 0x11},
	}
	_ = switcher.WriteRTP("h", pktH2)

	if switcher.lastOutSeq != 105 {
		t.Fatalf("Expected rewritten lastOutSeq 105, got %d", switcher.lastOutSeq)
	}
}

func TestTrackSwitcher_SimulcastTracks(t *testing.T) {
	outTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000},
		"video_viewer",
		"stream_viewer",
	)
	if err != nil {
		t.Fatalf("Failed to create outTrack: %v", err)
	}

	switcher := NewTrackSwitcher(outTrack, LayerMedium)
	if switcher.GetOutTrack() != outTrack {
		t.Fatalf("Expected GetOutTrack to return outTrack")
	}

	// Test SetIncomingTrack / GetIncomingTrack
	switcher.SetIncomingTrack("q", nil)
	switcher.SetIncomingTrack("h", nil)
	switcher.SetIncomingTrack("f", nil)

	if switcher.GetIncomingTrack("q") != nil {
		t.Fatalf("Expected nil for trackQ")
	}

	// Test ActiveTrack getter and setter
	if switcher.GetActiveTrack() != nil {
		t.Fatalf("Expected nil for initial activeTrack")
	}
	switcher.SetActiveTrack(nil)
	if switcher.GetActiveTrack() != nil {
		t.Fatalf("Expected nil after SetActiveTrack(nil)")
	}
}

func TestTrackSwitcher_PendingSwitchAndTargetTrack(t *testing.T) {
	outTrack, _ := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000},
		"video_viewer",
		"stream_viewer",
	)

	switcher := NewTrackSwitcher(outTrack, LayerMedium)

	// Initially no pending switch
	if switcher.IsPendingSwitch() {
		t.Fatalf("expected pendingSwitch to be false initially")
	}

	// Switch layer to Low ('q')
	switcher.SwitchLayer(LayerLow)
	if !switcher.IsPendingSwitch() {
		t.Fatalf("expected pendingSwitch to be true after SwitchLayer")
	}
	if switcher.GetTargetLayer() != LayerLow {
		t.Fatalf("expected targetLayer to be 'q', got '%s'", switcher.GetTargetLayer())
	}
}

func TestIsKeyframe(t *testing.T) {
	// VP8 Keyframe: first byte 0x10 (X=0), payload header byte with bit0=0 (0x00) -> Keyframe
	vp8Keyframe := []byte{0x10, 0x00, 0x00}
	if !IsKeyframe(webrtc.MimeTypeVP8, vp8Keyframe) {
		t.Fatalf("expected VP8 keyframe to be recognized")
	}

	// VP8 P-frame: first byte 0x10, payload header byte with bit0=1 (0x01) -> Interframe
	vp8Pframe := []byte{0x10, 0x01, 0x00}
	if IsKeyframe(webrtc.MimeTypeVP8, vp8Pframe) {
		t.Fatalf("expected VP8 P-frame not to be keyframe")
	}

	// H264 IDR Slice (NAL type 5): byte 0x05
	h264Keyframe := []byte{0x05, 0x88, 0x00}
	if !IsKeyframe(webrtc.MimeTypeH264, h264Keyframe) {
		t.Fatalf("expected H264 IDR keyframe to be recognized")
	}

	// H264 Non-IDR Slice (NAL type 1): byte 0x01
	h264Pframe := []byte{0x01, 0x88, 0x00}
	if IsKeyframe(webrtc.MimeTypeH264, h264Pframe) {
		t.Fatalf("expected H264 non-IDR frame not to be keyframe")
	}
}

func TestIsVP8Keyframe(t *testing.T) {
	// Empty payload -> false
	if IsVP8Keyframe(nil) || IsVP8Keyframe([]byte{}) {
		t.Fatalf("expected empty payload to return false")
	}

	// VP8 Keyframe without extended descriptor (X=0): 1st byte 0x10, 2nd byte 0x00 (bit0=0)
	keyframeSimple := []byte{0x10, 0x00, 0x00, 0xAA, 0xBB}
	if !IsVP8Keyframe(keyframeSimple) {
		t.Fatalf("expected keyframeSimple to return true")
	}

	// VP8 Interframe (P-frame) without extended descriptor: 1st byte 0x10, 2nd byte 0x01 (bit0=1)
	pframeSimple := []byte{0x10, 0x01, 0x00, 0xAA, 0xBB}
	if IsVP8Keyframe(pframeSimple) {
		t.Fatalf("expected pframeSimple to return false")
	}

	// VP8 Keyframe with extended descriptor (X=1, I=1):
	// Byte 0: 0x90 (X=1, S=1)
	// Byte 1: 0x80 (I=1)
	// Byte 2: 0x15 (PictureID)
	// Byte 3: 0x00 (Frame header: bit0=0 Keyframe)
	keyframeExtended := []byte{0x90, 0x80, 0x15, 0x00, 0x11, 0x22}
	if !IsVP8Keyframe(keyframeExtended) {
		t.Fatalf("expected keyframeExtended to return true")
	}
}
