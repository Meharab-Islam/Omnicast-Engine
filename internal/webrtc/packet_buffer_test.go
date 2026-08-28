package webrtc

import (
	"sync"
	"testing"

	"github.com/pion/rtp"
)

func TestPacketBuffer_BasicOperations(t *testing.T) {
	pb := NewPacketBuffer(4)

	if pb.Capacity() != 4 {
		t.Fatalf("expected capacity 4, got %d", pb.Capacity())
	}
	if pb.Size() != 0 {
		t.Fatalf("expected size 0, got %d", pb.Size())
	}

	// Push 3 packets
	pb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 100}})
	pb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 101}})
	pb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 102}})

	if pb.Size() != 3 {
		t.Fatalf("expected size 3, got %d", pb.Size())
	}
	if pb.Head() != 3 {
		t.Fatalf("expected head index 3, got %d", pb.Head())
	}
	if pb.Tail() != 0 {
		t.Fatalf("expected tail index 0, got %d", pb.Tail())
	}

	// Lookup sequence numbers
	pkt, found := pb.GetBySequenceNumber(101)
	if !found || pkt.SequenceNumber != 101 {
		t.Fatalf("expected to find seq 101, got %v", pkt)
	}

	_, foundMissing := pb.GetBySequenceNumber(999)
	if foundMissing {
		t.Fatalf("expected not to find seq 999")
	}

	// Pop one from tail
	popped := pb.Pop()
	if popped == nil || popped.SequenceNumber != 100 {
		t.Fatalf("expected popped seq 100, got %v", popped)
	}
	if pb.Size() != 2 {
		t.Fatalf("expected size 2, got %d", pb.Size())
	}
	if pb.Tail() != 1 {
		t.Fatalf("expected tail index 1, got %d", pb.Tail())
	}
}

func TestPacketBuffer_CircularOverflow(t *testing.T) {
	pb := NewPacketBuffer(3)

	// Fill buffer
	pb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1}})
	pb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 2}})
	pb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 3}})

	if pb.Size() != 3 {
		t.Fatalf("expected size 3, got %d", pb.Size())
	}
	if pb.Head() != 0 { // 3 % 3 = 0
		t.Fatalf("expected head 0, got %d", pb.Head())
	}
	if pb.Tail() != 0 {
		t.Fatalf("expected tail 0, got %d", pb.Tail())
	}

	// Overflow: pushing 4th packet overwrites seq 1 at index 0 and advances tail to 1
	pb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 4}})

	if pb.Size() != 3 {
		t.Fatalf("expected size 3 after overwrite, got %d", pb.Size())
	}
	if pb.Head() != 1 {
		t.Fatalf("expected head 1, got %d", pb.Head())
	}
	if pb.Tail() != 1 {
		t.Fatalf("expected tail 1, got %d", pb.Tail())
	}

	// Seq 1 should be overwritten -> Get returns nil
	pkt1 := pb.Get(1)
	if pkt1 != nil {
		t.Fatalf("expected seq 1 to be overwritten and return nil, got %v", pkt1)
	}

	// Seq 2, 3, 4 should be found via Get
	pkt4 := pb.Get(4)
	if pkt4 == nil || pkt4.SequenceNumber != 4 {
		t.Fatalf("expected seq 4 found, got %v", pkt4)
	}

	pkt2 := pb.Get(2)
	if pkt2 == nil || pkt2.SequenceNumber != 2 {
		t.Fatalf("expected seq 2 found, got %v", pkt2)
	}
}

func TestPacketBuffer_ConcurrentAccess(t *testing.T) {
	pb := NewPacketBuffer(64)
	var wg sync.WaitGroup

	// Concurrent Writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(base uint16) {
			defer wg.Done()
			for j := uint16(0); j < 50; j++ {
				pb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: base + j}})
			}
		}(uint16(i * 100))
	}

	// Concurrent Readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, _ = pb.GetBySequenceNumber(uint16(j))
				_ = pb.Size()
			}
		}()
	}

	wg.Wait()
}
