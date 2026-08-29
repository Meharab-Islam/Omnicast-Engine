package webrtc

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/rtp"
	pionWebRTC "github.com/pion/webrtc/v3"
	"omnicast/internal/models"
)

func TestUDPRTPForwarder_And_Receiver(t *testing.T) {
	// 1. Initialize Receiver on dynamic port (e.g. 51234)
	receiver, err := NewUDPRTPReceiver(51234, nil)
	if err != nil {
		t.Fatalf("Failed to create UDPRTPReceiver: %v", err)
	}
	defer receiver.Close()

	var receivedCount int32
	var lastSeq uint16

	receiver.SetPacketHandler(func(pkt *rtp.Packet) {
		atomic.AddInt32(&receivedCount, 1)
		lastSeq = pkt.Header.SequenceNumber
	})

	// 2. Initialize Forwarder on Node A
	forwarder, err := NewUDPRTPForwarder()
	if err != nil {
		t.Fatalf("Failed to create UDPRTPForwarder: %v", err)
	}
	defer forwarder.Close()

	// 3. Add Node B as destination
	err = forwarder.AddEdgeNode("node-B", "127.0.0.1", 51234)
	if err != nil {
		t.Fatalf("Failed to add edge node: %v", err)
	}

	// 4. Forward RTP packets from Node A -> Node B
	for i := uint16(1); i <= 5; i++ {
		pkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    96,
				SequenceNumber: i,
				Timestamp:      uint32(i * 3000),
				SSRC:           0x12345678,
			},
			Payload: []byte{0x00, 0x01, 0x02, byte(i)},
		}
		if err := forwarder.ForwardRTP(pkt); err != nil {
			t.Fatalf("Failed to forward RTP packet %d: %v", i, err)
		}
	}

	// 5. Verify packets received on Node B
	time.Sleep(100 * time.Millisecond)

	count := atomic.LoadInt32(&receivedCount)
	if count != 5 {
		t.Fatalf("expected 5 packets received on Node B, got %d", count)
	}
	if lastSeq != 5 {
		t.Fatalf("expected last sequence number 5, got %d", lastSeq)
	}

	// 6. Test RemoveEdgeNode
	forwarder.RemoveEdgeNode("node-B")
	pktExtra := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 6,
			Timestamp:      18000,
		},
		Payload: []byte{0xAA},
	}
	_ = forwarder.ForwardRTP(pktExtra)

	time.Sleep(50 * time.Millisecond)
	if countAfter := atomic.LoadInt32(&receivedCount); countAfter != 5 {
		t.Fatalf("expected count to remain 5 after removing node-B, got %d", countAfter)
	}
}

func TestUDPRTPReceiver_LocalFanOutToViewers(t *testing.T) {
	// Create Room on Edge Node B
	room := models.NewRoom("room-fanout-1", "host-alice")
	track, _ := pionWebRTC.NewTrackLocalStaticRTP(pionWebRTC.RTPCodecCapability{MimeType: pionWebRTC.MimeTypeVP8}, "video", "pion")
	room.VideoTrack = track

	// Initialize TrackSwitcher for a local viewer on Node B
	outTrack, _ := pionWebRTC.NewTrackLocalStaticRTP(pionWebRTC.RTPCodecCapability{MimeType: pionWebRTC.MimeTypeVP8}, "video_out", "pion")
	switcher := NewTrackSwitcher(outTrack, "high")
	room.RegisterTrackSwitcher("viewer-bob", switcher)

	// Initialize Receiver on Edge Node B bound to the room
	receiver, err := NewUDPRTPReceiver(51235, room)
	if err != nil {
		t.Fatalf("Failed to create receiver: %v", err)
	}
	defer receiver.Close()

	// Forwarder on Node A
	forwarder, err := NewUDPRTPForwarder()
	if err != nil {
		t.Fatalf("Failed to create forwarder: %v", err)
	}
	defer forwarder.Close()

	_ = forwarder.AddEdgeNode("node-B", "127.0.0.1", 51235)

	// Send video packet from Node A -> Node B
	pkt := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 100,
			Timestamp:      90000,
			SSRC:           0x8888,
		},
		Payload: []byte{0x00, 0x01},
	}
	_ = forwarder.ForwardRTP(pkt)

	time.Sleep(100 * time.Millisecond)

	// Verify local track switcher received and processed the packet on Node B
	if switcher.GetCurrentLayer() != "high" {
		t.Fatalf("expected switcher layer high, got %s", switcher.GetCurrentLayer())
	}
}

