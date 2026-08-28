package webrtc

import (
	"sync"
	"testing"
	"time"
)

func TestPLIThrottler_BasicThrottling(t *testing.T) {
	throttler := NewPLIThrottler(50 * time.Millisecond)

	trackID := "video-track-1"

	// 1st request -> allowed
	if !throttler.CanSendPLI(trackID) {
		t.Fatalf("expected 1st PLI request to be allowed")
	}

	// Immediate 2nd request -> throttled
	if throttler.CanSendPLI(trackID) {
		t.Fatalf("expected immediate 2nd PLI request to be throttled")
	}

	// Another track ID -> allowed (independent per track)
	otherTrackID := "video-track-2"
	if !throttler.CanSendPLI(otherTrackID) {
		t.Fatalf("expected PLI for other track to be allowed")
	}

	// Wait for interval to elapse
	time.Sleep(60 * time.Millisecond)

	// 3rd request after cooldown -> allowed
	if !throttler.CanSendPLI(trackID) {
		t.Fatalf("expected PLI request after cooldown to be allowed")
	}
}

func TestPLIThrottler_ResetAndClear(t *testing.T) {
	throttler := NewPLIThrottler(1 * time.Second)
	trackID := "video-track-A"

	if !throttler.ShouldSend(trackID) {
		t.Fatalf("expected initial request to be allowed")
	}
	if throttler.ShouldSend(trackID) {
		t.Fatalf("expected immediate request to be throttled")
	}

	// Reset track
	throttler.Reset(trackID)
	if !throttler.ShouldSend(trackID) {
		t.Fatalf("expected request after Reset to be allowed")
	}

	// Clear all
	throttler.Clear()
	if !throttler.ShouldSend(trackID) {
		t.Fatalf("expected request after Clear to be allowed")
	}
}

func TestPLIThrottler_ConcurrentAccess(t *testing.T) {
	throttler := NewPLIThrottler(10 * time.Millisecond)
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			trackID := "track-concurrent"
			for j := 0; j < 10; j++ {
				_ = throttler.ShouldSend(trackID)
				_ = throttler.LastSentTime(trackID)
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
}
