package webrtc

import (
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

func TestLeakyBucketPacer(t *testing.T) {
	pacer := NewLeakyBucketPacer(5000000, 100)
	defer pacer.Stop()

	pacer.SetBitrate(8000000)

	track, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000},
		"test-video-track",
		"test-stream",
	)
	if err != nil {
		t.Fatalf("Failed to create track: %v", err)
	}

	pkt := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 100,
			Timestamp:      90000,
			SSRC:           12345,
		},
		Payload: []byte{0x01, 0x02, 0x03, 0x04},
	}

	// Enqueue packet
	pacer.Enqueue(track, pkt)

	// Allow pacer loop to process
	time.Sleep(10 * time.Millisecond)
}
