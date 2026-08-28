package webrtc

import (
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

func TestSharedPacket_RetainAndRelease(t *testing.T) {
	released := false
	sp := &SharedPacket{
		refCount: 1,
		onRelease: func(p *SharedPacket) {
			released = true
		},
	}

	sp.Retain()
	if sp.RefCount() != 2 {
		t.Fatalf("expected refCount 2, got %d", sp.RefCount())
	}

	sp.Release()
	if released {
		t.Fatalf("expected packet not to be released yet at refCount 1")
	}

	sp.Release()
	if !released {
		t.Fatalf("expected packet to be released when refCount hits 0")
	}
}

func TestFanOutDispatcher_SubscribeAndBroadcast(t *testing.T) {
	dispatcher := NewFanOutDispatcher(4, 256)
	defer dispatcher.Stop()

	// Create 10 dummy subscribers
	track, _ := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000},
		"test_video",
		"test_stream",
	)

	subscribers := make([]*Subscriber, 10)
	for i := 0; i < 10; i++ {
		sub := NewSubscriber(string(rune('A'+i)), track, 32)
		_ = dispatcher.Subscribe(sub)
		subscribers[i] = sub
	}

	if count := dispatcher.GetSubscriberCount(); count != 10 {
		t.Fatalf("expected 10 subscribers, got %d", count)
	}

	// Broadcast an RTP packet
	pkt := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 100,
			Timestamp:      90000,
		},
		Payload: []byte{0x10, 0x00, 0x01, 0x02},
	}

	sent := dispatcher.Broadcast(pkt, []byte{1, 2, 3}, nil)
	if sent != 10 {
		t.Fatalf("expected 10 packets dispatched, got %d", sent)
	}

	// Read from subscribers
	time.Sleep(50 * time.Millisecond)
	for i, sub := range subscribers {
		select {
		case p := <-sub.Queue:
			if p.Header.SequenceNumber != 100 {
				t.Fatalf("subscriber %d received wrong seq %d", i, p.Header.SequenceNumber)
			}
			p.Release()
		default:
			t.Fatalf("subscriber %d queue empty", i)
		}
	}
}

func TestFanOutDispatcher_SelectiveFiltering(t *testing.T) {
	dispatcher := NewFanOutDispatcher(4, 256)
	defer dispatcher.Stop()

	track, _ := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000},
		"test_video",
		"test_stream",
	)

	sub1 := NewSubscriber("viewer_1", track, 32)
	sub2 := NewSubscriber("viewer_2", track, 32)
	_ = dispatcher.Subscribe(sub1)
	_ = dispatcher.Subscribe(sub2)

	pkt := &rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 200},
		Payload: []byte{0xAA, 0xBB},
	}

	// Filter: only viewer_1 is visible (viewer_2 paused)
	sent := dispatcher.Broadcast(pkt, nil, func(subID string) bool {
		return subID == "viewer_1"
	})

	if sent != 1 {
		t.Fatalf("expected 1 packet dispatched with filter, got %d", sent)
	}

	time.Sleep(50 * time.Millisecond)

	// sub1 should have the packet
	select {
	case p := <-sub1.Queue:
		p.Release()
	default:
		t.Fatalf("expected sub1 to receive packet")
	}

	// sub2 should NOT have the packet
	select {
	case <-sub2.Queue:
		t.Fatalf("expected sub2 to not receive packet")
	default:
		// OK
	}
}

func TestFanOutDispatcher_Unsubscribe(t *testing.T) {
	dispatcher := NewFanOutDispatcher(2, 64)
	defer dispatcher.Stop()

	sub := NewSubscriber("v1", nil, 10)
	_ = dispatcher.Subscribe(sub)

	if dispatcher.GetSubscriberCount() != 1 {
		t.Fatalf("expected 1 subscriber")
	}

	dispatcher.Unsubscribe("v1")
	if dispatcher.GetSubscriberCount() != 0 {
		t.Fatalf("expected 0 subscribers after unsubscribe")
	}
}
