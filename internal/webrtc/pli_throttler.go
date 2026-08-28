package webrtc

import (
	"sync"
	"time"
)

// DefaultPLIInterval defines the minimum cooldown between consecutive PLI requests for a single track (1 second)
const DefaultPLIInterval = 1 * time.Second

// GlobalPLIThrottler is the shared PLIThrottler instance for all WebRTC tracks across all rooms
var GlobalPLIThrottler = NewPLIThrottler(DefaultPLIInterval)

// CanSendPLI is a package-level helper that delegates to GlobalPLIThrottler
func CanSendPLI(trackID string) bool {
	return GlobalPLIThrottler.CanSendPLI(trackID)
}

// ForceSendPLI is a package-level helper that forcefully updates the throttler timestamp for trackID
func ForceSendPLI(trackID string) {
	GlobalPLIThrottler.ForceSendPLI(trackID)
}

// PLIThrottler manages rate-limiting and debouncing of Picture Loss Indication (Keyframe) requests
// on a per-track basis to prevent encoder CPU thrashing and upstream bandwidth flooding.
type PLIThrottler struct {
	mu          sync.Mutex
	lastSent    map[string]time.Time
	minInterval time.Duration
}

// NewPLIThrottler creates and initializes a new PLIThrottler with a specified minimum cooldown interval
func NewPLIThrottler(minInterval time.Duration) *PLIThrottler {
	if minInterval <= 0 {
		minInterval = DefaultPLIInterval
	}
	return &PLIThrottler{
		lastSent:    make(map[string]time.Time),
		minInterval: minInterval,
	}
}

// CanSendPLI checks whether a PLI keyframe request can be sent for the specified trackID.
// It is secured with sync.Mutex:
// 1. Checks the map for trackID. If less than 1000 milliseconds have passed since last sent time, returns false.
// 2. If 1000 milliseconds have passed (or first request for trackID), updates the map with time.Now() and returns true.
func (pt *PLIThrottler) CanSendPLI(trackID string) bool {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	now := time.Now()
	if last, exists := pt.lastSent[trackID]; exists {
		cooldown := pt.minInterval
		if cooldown <= 0 {
			cooldown = 1000 * time.Millisecond
		}
		if now.Sub(last) < cooldown {
			return false // Less than 1000 milliseconds since last sent time
		}
	}

	// If 1000 milliseconds have passed, update map with time.Now() for trackID and return true
	pt.lastSent[trackID] = now
	return true
}

// ShouldSend is an alias for CanSendPLI
func (pt *PLIThrottler) ShouldSend(trackID string) bool {
	return pt.CanSendPLI(trackID)
}

// ForceSendPLI forcefully updates the timestamp for trackID, bypassing cooldown restrictions,
// and ensures an immediate keyframe can be requested for a newly joined viewer.
func (pt *PLIThrottler) ForceSendPLI(trackID string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.lastSent[trackID] = time.Now()
}

// Reset clears the recorded timestamp for a specific trackID, allowing the next PLI to send immediately
func (pt *PLIThrottler) Reset(trackID string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	delete(pt.lastSent, trackID)
}

// Clear wipes all recorded track timestamps from the throttler
func (pt *PLIThrottler) Clear() {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.lastSent = make(map[string]time.Time)
}

// LastSentTime returns the last recorded timestamp for the given trackID (or zero time if not present)
func (pt *PLIThrottler) LastSentTime(trackID string) time.Time {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	return pt.lastSent[trackID]
}
