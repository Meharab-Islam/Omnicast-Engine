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

	// Packets from 'q' while pending switch should still be processed until 'h' arrives
	pkt3 := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 102,
			Timestamp:      7000,
			SSRC:           11111,
		},
		Payload: []byte{0x90, 0x80, 0x03},
	}
	_ = switcher.WriteRTP("q", pkt3)

	// 3. First packet from new layer 'h' (which has totally different base sequence number & timestamp)
	pktH1 := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 5000, // Different sequence space!
			Timestamp:      999999, // Different timestamp space!
			SSRC:           22222,
		},
		Payload: []byte{0x90, 0x80, 0x10},
	}
	_ = switcher.WriteRTP("h", pktH1)

	if switcher.GetCurrentLayer() != "h" {
		t.Fatalf("Expected current layer to switch to 'h', got '%s'", switcher.GetCurrentLayer())
	}

	// Check that last output sequence number is monotonic (should be 103, continuing from 102)
	if switcher.lastOutSeq != 103 {
		t.Fatalf("Expected rewritten lastOutSeq 103, got %d", switcher.lastOutSeq)
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

	if switcher.lastOutSeq != 104 {
		t.Fatalf("Expected rewritten lastOutSeq 104, got %d", switcher.lastOutSeq)
	}
}
