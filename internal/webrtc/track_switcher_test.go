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

	// 1. Send packets on layer 'q' (pkt1 is initial Keyframe)
	pkt1 := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 100,
			Timestamp:      1000,
			SSRC:           11111,
		},
		Payload: []byte{0x10, 0x00, 0x00}, // VP8 Keyframe
	}
	_ = switcher.WriteRTP("q", pkt1)

	pkt2 := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 101,
			Timestamp:      4000,
			SSRC:           11111,
		},
		Payload: []byte{0x10, 0x01, 0x02},
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

func TestTrackSwitcher_VP9SingleTrackSVC(t *testing.T) {
	outTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP9, ClockRate: 90000},
		"video",
		"stream",
	)
	if err != nil {
		t.Fatalf("Failed to create outTrack: %v", err)
	}

	switcher := NewTrackSwitcher(outTrack, "q") // starts at S=0 (Low)
	if switcher.GetSpatialLayer() != 0 {
		t.Fatalf("expected initial spatial layer 0, got %d", switcher.GetSpatialLayer())
	}

	// 1. Send S0 (Low) packet -> should forward
	// Byte 0: I=0, P=0 (Keyframe), L=1 (0x20), F=0, B=1 (0x08) -> 0x28
	// Byte 1: Layer indices: TID=0, U=0, SID=0 (0x00) -> 0x00
	// Byte 2: TL0PICIDX -> 0x01
	pktS0 := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 100,
			Timestamp:      1000,
		},
		Payload: []byte{0x28, 0x00, 0x01, 0xAA},
	}
	_ = switcher.WriteRTP("default", pktS0)

	// 2. Send S1 (Medium) packet while current layer is S0 -> should be DROPPED
	// Byte 1: SID=1 -> 1<<1 = 0x02
	pktS1Dropped := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 101,
			Timestamp:      1000,
		},
		Payload: []byte{0x28, 0x02, 0x01, 0xBB},
	}
	_ = switcher.WriteRTP("default", pktS1Dropped)

	// 3. Switch to Medium layer ('h' -> S1)
	switcher.SwitchLayer("h")
	if switcher.GetTargetLayer() != "h" {
		t.Fatalf("expected target layer 'h'")
	}

	// Send an S1 P-frame (P=1 -> 0x68) while waiting for keyframe/up-switch -> should be dropped
	pktS1Pframe := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 102,
			Timestamp:      4000,
		},
		Payload: []byte{0x68, 0x02, 0x02, 0xCC},
	}
	_ = switcher.WriteRTP("default", pktS1Pframe)

	if switcher.GetSpatialLayer() != 0 {
		t.Fatalf("expected switcher to remain on S0 until keyframe/up-switch, got %d", switcher.GetSpatialLayer())
	}

	// Send S1 Keyframe (P=0, B=1 -> 0x28, SID=1 -> 0x02) -> should trigger up-switch to S1!
	pktS1Keyframe := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 103,
			Timestamp:      7000,
		},
		Payload: []byte{0x28, 0x02, 0x03, 0xDD},
	}
	_ = switcher.WriteRTP("default", pktS1Keyframe)

	if switcher.GetSpatialLayer() != 1 {
		t.Fatalf("expected switcher to switch to S1 on keyframe, got %d", switcher.GetSpatialLayer())
	}
	if switcher.GetCurrentLayer() != "h" {
		t.Fatalf("expected current layer 'h', got '%s'", switcher.GetCurrentLayer())
	}
}

func TestTrackSwitcher_DropHighestSpatialLayerOnCongestion(t *testing.T) {
	outTrack, _ := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP9, ClockRate: 90000},
		"video",
		"stream",
	)

	switcher := NewTrackSwitcher(outTrack, "f") // Starts at S=2 (High / Full)
	if switcher.GetSpatialLayer() != 2 {
		t.Fatalf("expected initial spatial layer 2, got %d", switcher.GetSpatialLayer())
	}

	// 1. Trigger Congestion (loss > 5%) -> should drop S=2 down to S=1
	handled := switcher.HandleCongestion(6.5, 800_000)
	if !handled {
		t.Fatalf("expected HandleCongestion to return true")
	}
	if switcher.GetSpatialLayer() != 1 {
		t.Fatalf("expected spatial layer dropped to 1, got %d", switcher.GetSpatialLayer())
	}
	if switcher.GetCurrentLayer() != "h" {
		t.Fatalf("expected current layer 'h', got '%s'", switcher.GetCurrentLayer())
	}

	// 2. Severe Congestion (loss > 15%) -> should drop S=1 down to S=0
	handled = switcher.HandleCongestion(18.0, 300_000)
	if !handled {
		t.Fatalf("expected HandleCongestion to return true on second drop")
	}
	if switcher.GetSpatialLayer() != 0 {
		t.Fatalf("expected spatial layer dropped to 0, got %d", switcher.GetSpatialLayer())
	}
	if switcher.GetCurrentLayer() != "q" {
		t.Fatalf("expected current layer 'q', got '%s'", switcher.GetCurrentLayer())
	}

	// 3. Already at S=0 -> cannot drop further
	handled = switcher.HandleCongestion(20.0, 200_000)
	if handled {
		t.Fatalf("expected HandleCongestion to return false when already at S=0")
	}
}

func TestTrackSwitcher_TemporalLayerDropping(t *testing.T) {
	outTrack, _ := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP9, ClockRate: 90000},
		"video",
		"stream",
	)

	switcher := NewTrackSwitcher(outTrack, "f") // Starts at S=2, T=2 (Full 30fps)
	if switcher.GetTemporalLayer() != 2 {
		t.Fatalf("expected initial temporal layer 2, got %d", switcher.GetTemporalLayer())
	}

	// 1. Send packet with T=0 (Base layer) -> should forward
	// Byte 0: I=0, P=0, L=1 (0x20), B=1 (0x08) -> 0x28
	// Byte 1: TID=0 (0x00), U=0, SID=2 (2<<1 = 0x04) -> 0x04
	// Byte 2: TL0PICIDX -> 0x01
	pktT0 := &rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 100, Timestamp: 1000},
		Payload: []byte{0x28, 0x04, 0x01, 0xAA},
	}
	_ = switcher.WriteRTP("default", pktT0)

	// 2. Drop highest temporal layer (drop T=2, keeping T=0 and T=1) due to slight constraint
	newT := switcher.DropHighestTemporalLayer()
	if newT != 1 || switcher.GetTemporalLayer() != 1 {
		t.Fatalf("expected temporal layer dropped to 1, got %d", switcher.GetTemporalLayer())
	}

	// 3. Packet with T=2 -> should be DROPPED
	// Byte 1: TID=2 (2<<5 = 0x40), SID=2 (0x04) -> 0x44
	pktT2 := &rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 101, Timestamp: 2000},
		Payload: []byte{0x28, 0x44, 0x01, 0xBB},
	}
	_ = switcher.WriteRTP("default", pktT2)

	// 4. Packet with T=1 -> should be FORWARDED
	// Byte 1: TID=1 (1<<5 = 0x20), SID=2 (0x04) -> 0x24
	pktT1 := &rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 102, Timestamp: 3000},
		Payload: []byte{0x28, 0x24, 0x01, 0xCC},
	}
	_ = switcher.WriteRTP("default", pktT1)

	// 5. HandleTemporalConstraint test
	handled := switcher.HandleTemporalConstraint(true)
	if !handled {
		t.Fatalf("expected HandleTemporalConstraint to drop T=1 to T=0")
	}
	if switcher.GetTemporalLayer() != 0 {
		t.Fatalf("expected temporal layer 0, got %d", switcher.GetTemporalLayer())
	}
}

func TestTrackSwitcher_ContiguousSequenceNumbersOnSVCDrop(t *testing.T) {
	outTrack, _ := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP9, ClockRate: 90000},
		"video",
		"stream",
	)

	switcher := NewTrackSwitcher(outTrack, "f") // Starts at S=2, T=2
	// Drop T=2 -> only forward T=0 and T=1
	switcher.SetTemporalLayer(1)

	// Feed packets where odd sequence numbers have T=2 (which get dropped):
	// Packet 1: inSeq 100, T=0 -> FORWARDED (outSeq 100)
	// Packet 2: inSeq 101, T=2 -> DROPPED
	// Packet 3: inSeq 102, T=1 -> FORWARDED (outSeq 101)
	// Packet 4: inSeq 103, T=2 -> DROPPED
	// Packet 5: inSeq 104, T=0 -> FORWARDED (outSeq 102)

	pkt1 := &rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 100, Timestamp: 1000},
		Payload: []byte{0x28, 0x04, 0x01, 0xAA}, // T=0, SID=2
	}
	_ = switcher.WriteRTP("default", pkt1)

	pkt2 := &rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 101, Timestamp: 2000},
		Payload: []byte{0x28, 0x44, 0x01, 0xBB}, // T=2 (dropped)
	}
	_ = switcher.WriteRTP("default", pkt2)

	pkt3 := &rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 102, Timestamp: 3000},
		Payload: []byte{0x28, 0x24, 0x01, 0xCC}, // T=1, SID=2
	}
	_ = switcher.WriteRTP("default", pkt3)

	pkt4 := &rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 103, Timestamp: 4000},
		Payload: []byte{0x28, 0x44, 0x01, 0xDD}, // T=2 (dropped)
	}
	_ = switcher.WriteRTP("default", pkt4)

	pkt5 := &rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 104, Timestamp: 5000},
		Payload: []byte{0x28, 0x04, 0x01, 0xEE}, // T=0, SID=2
	}
	_ = switcher.WriteRTP("default", pkt5)

	// Verify that the switcher's lastOutSeq is 102 (strictly 100, 101, 102 without gaps!)
	if switcher.lastOutSeq != 102 {
		t.Fatalf("expected lastOutSeq to be strictly contiguous 102, got %d", switcher.lastOutSeq)
	}
}

func TestTrackSwitcher_DropDeltaFramesUntilKeyframeOnSubscribe(t *testing.T) {
	outTrack, _ := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000},
		"video",
		"stream",
	)

	switcher := NewTrackSwitcher(outTrack, "f")

	// 1. Send 5 Delta frames (H.264 Non-IDR Slice NAL type 1 = 0x01) on subscribe
	// Switcher MUST drop all of them and not start forwarding!
	for i := uint16(100); i < 105; i++ {
		deltaPkt := &rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: i,
				Timestamp:      uint32(i) * 3000,
				SSRC:           9999,
			},
			Payload: []byte{0x01, 0x88, 0x00}, // H264 P-frame (NAL 1)
		}
		_ = switcher.WriteRTP("f", deltaPkt)
	}

	if switcher.hasReceivedKeyframe {
		t.Fatalf("Expected hasReceivedKeyframe to remain false after receiving delta frames")
	}
	if switcher.started {
		t.Fatalf("Expected switcher not to start forwarding on delta frames")
	}

	// 2. Send H.264 IDR Keyframe (NAL type 5 = 0x05)
	keyPkt := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 105,
			Timestamp:      105 * 3000,
			SSRC:           9999,
		},
		Payload: []byte{0x05, 0x88, 0x00}, // H264 IDR Keyframe (NAL 5)
	}
	_ = switcher.WriteRTP("f", keyPkt)

	if !switcher.hasReceivedKeyframe {
		t.Fatalf("Expected hasReceivedKeyframe to become true after receiving IDR keyframe")
	}
	if !switcher.started {
		t.Fatalf("Expected switcher to start forwarding cleanly on keyframe")
	}
}





