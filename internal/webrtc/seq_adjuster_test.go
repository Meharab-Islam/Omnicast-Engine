package webrtc

import (
	"sync"
	"testing"
)

func TestSequenceNumberAdjuster_BasicAndContinuity(t *testing.T) {
	adj := NewSequenceNumberAdjuster()

	if adj.GetOffset() != 0 {
		t.Fatalf("expected initial offset 0, got %d", adj.GetOffset())
	}
	if adj.IsInitialized() {
		t.Fatalf("expected not initialized")
	}

	// Stream 1: input seq 100, 101, 102
	out1 := adj.Adjust(100)
	if out1 != 100 {
		t.Fatalf("expected out1 100, got %d", out1)
	}
	out2 := adj.Adjust(101)
	if out2 != 101 {
		t.Fatalf("expected out2 101, got %d", out2)
	}
	out3 := adj.Adjust(102)
	if out3 != 102 {
		t.Fatalf("expected out3 102, got %d", out3)
	}

	if adj.LastInSeq() != 102 || adj.LastOutSeq() != 102 {
		t.Fatalf("expected lastInSeq 102 and lastOutSeq 102")
	}

	// Stream switch: new layer starts at seq 5000
	adj.Switch(5000)

	// Next output should be 103
	out4 := adj.Adjust(5000)
	if out4 != 103 {
		t.Fatalf("expected out4 103 after switch, got %d", out4)
	}
	out5 := adj.Adjust(5001)
	if out5 != 104 {
		t.Fatalf("expected out5 104, got %d", out5)
	}

	// SetOffset explicitly
	adj.SetOffset(20)
	if adj.GetOffset() != 20 {
		t.Fatalf("expected offset 20, got %d", adj.GetOffset())
	}
	if adj.Rewrite(10) != 30 {
		t.Fatalf("expected rewrite(10) with offset 20 to be 30, got %d", adj.Rewrite(10))
	}

	// Reset
	adj.Reset()
	if adj.IsInitialized() || adj.GetOffset() != 0 {
		t.Fatalf("expected reset state")
	}
}

func TestSequenceNumberAdjuster_WrapAround(t *testing.T) {
	adj := NewSequenceNumberAdjuster()

	// Seq near uint16 max
	adj.Adjust(65534)
	out := adj.Adjust(65535)
	if out != 65535 {
		t.Fatalf("expected 65535, got %d", out)
	}

	// Wraparound to 0
	outWrap := adj.Adjust(0)
	if outWrap != 0 {
		t.Fatalf("expected 0, got %d", outWrap)
	}
}

func TestSequenceNumberAdjuster_Concurrent(t *testing.T) {
	adj := NewSequenceNumberAdjuster()
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				adj.Adjust(uint16(id*100 + j))
				_ = adj.GetOffset()
				_ = adj.LastInSeq()
				_ = adj.LastOutSeq()
				_ = adj.Rewrite(uint16(j))
			}
		}(i)
	}

	wg.Wait()
}
