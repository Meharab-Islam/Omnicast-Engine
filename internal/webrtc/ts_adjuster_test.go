package webrtc

import (
	"sync"
	"testing"
)

func TestTimestampAdjuster_BasicAndContinuity(t *testing.T) {
	adj := NewTimestampAdjuster()

	if adj.GetOffset() != 0 {
		t.Fatalf("expected initial offset 0, got %d", adj.GetOffset())
	}
	if adj.IsInitialized() {
		t.Fatalf("expected not initialized")
	}

	// Stream 1 (e.g. 30fps video at 90kHz clock rate): timestamps 90000, 93000, 96000
	out1 := adj.Adjust(90000)
	if out1 != 90000 {
		t.Fatalf("expected out1 90000, got %d", out1)
	}
	out2 := adj.Adjust(93000)
	if out2 != 93000 {
		t.Fatalf("expected out2 93000, got %d", out2)
	}
	out3 := adj.Adjust(96000)
	if out3 != 96000 {
		t.Fatalf("expected out3 96000, got %d", out3)
	}

	if adj.LastInTS() != 96000 || adj.LastOutTS() != 96000 {
		t.Fatalf("expected lastInTS 96000 and lastOutTS 96000")
	}

	// Switch layer to new stream starting at timestamp 5,000,000
	adj.Switch(5000000, 3000)

	// Next output timestamp should be continuous: 96000 + 3000 = 99000
	out4 := adj.Adjust(5000000)
	if out4 != 99000 {
		t.Fatalf("expected out4 99000 after switch, got %d", out4)
	}
	out5 := adj.Adjust(5003000)
	if out5 != 102000 {
		t.Fatalf("expected out5 102000, got %d", out5)
	}

	// SetOffset explicitly
	adj.SetOffset(1000)
	if adj.GetOffset() != 1000 {
		t.Fatalf("expected offset 1000, got %d", adj.GetOffset())
	}
	if adj.Rewrite(500) != 1500 {
		t.Fatalf("expected rewrite(500) with offset 1000 to be 1500, got %d", adj.Rewrite(500))
	}

	// Reset
	adj.Reset()
	if adj.IsInitialized() || adj.GetOffset() != 0 {
		t.Fatalf("expected reset state")
	}
}

func TestTimestampAdjuster_WrapAround(t *testing.T) {
	adj := NewTimestampAdjuster()

	// Timestamp near uint32 max
	adj.Adjust(4294967000)
	out := adj.Adjust(4294967295)
	if out != 4294967295 {
		t.Fatalf("expected 4294967295, got %d", out)
	}

	// Wraparound
	outWrap := adj.Adjust(3000)
	if outWrap != 3000 {
		t.Fatalf("expected 3000, got %d", outWrap)
	}
}

func TestTimestampAdjuster_Concurrent(t *testing.T) {
	adj := NewTimestampAdjuster()
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				adj.Adjust(uint32(id*3000 + j*3000))
				_ = adj.GetOffset()
				_ = adj.LastInTS()
				_ = adj.LastOutTS()
				_ = adj.Rewrite(uint32(j * 3000))
			}
		}(i)
	}

	wg.Wait()
}
