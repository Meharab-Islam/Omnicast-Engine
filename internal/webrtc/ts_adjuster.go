package webrtc

import (
	"sync"
)

// DefaultFrameDuration90kHz represents the default RTP timestamp step for 30 FPS video at 90 kHz clock rate (90000 / 30 = 3000)
const DefaultFrameDuration90kHz uint32 = 3000

// TimestampAdjuster maintains an offset variable to rewrite RTP timestamps continuously
// across simulcast layer switches, stream transitions, or track replacements,
// ensuring the video playback clock does not jump, drift backwards, or freeze during a switch.
type TimestampAdjuster struct {
	mu          sync.RWMutex
	offset      uint32 // Offset added to incoming timestamps: outTS = inTS + offset
	lastInTS    uint32 // Last observed incoming timestamp
	lastOutTS   uint32 // Last emitted rewritten timestamp
	initialized bool   // Indicates whether at least one packet has been processed
}

// NewTimestampAdjuster creates and initializes a new TimestampAdjuster
func NewTimestampAdjuster() *TimestampAdjuster {
	return &TimestampAdjuster{
		offset: 0,
	}
}

// Adjust rewrites an incoming RTP timestamp using the current offset:
// outTS = inTS + offset
// It updates the internal lastInTS and lastOutTS states in a thread-safe manner.
func (t *TimestampAdjuster) Adjust(inTS uint32) uint32 {
	t.mu.Lock()
	defer t.mu.Unlock()

	outTS := inTS + t.offset
	t.lastInTS = inTS
	t.lastOutTS = outTS
	t.initialized = true
	return outTS
}

// AdjustContinuous smoothly advances output timestamps across dropped SVC layer frames or layer switches.
// Packets belonging to the same input frame (inTS == lastInTS) retain identical output timestamps,
// while new frames advance monotonically to maintain a stable clock for the decoder.
func (t *TimestampAdjuster) AdjustContinuous(inTS uint32, defaultStep uint32) uint32 {
	t.mu.Lock()
	defer t.mu.Unlock()

	if defaultStep == 0 {
		defaultStep = DefaultFrameDuration90kHz
	}

	if !t.initialized {
		t.lastInTS = inTS
		t.lastOutTS = inTS
		t.offset = 0
		t.initialized = true
		return t.lastOutTS
	}

	if inTS == t.lastInTS {
		// Same frame (multi-packet frame): output timestamp must match preceding packet
		return t.lastOutTS
	}

	// Calculate delta
	delta := inTS - t.lastInTS
	if delta > 90000 { // Large jump or discontinuity -> smooth with default step
		delta = defaultStep
	}

	t.lastInTS = inTS
	t.lastOutTS += delta
	t.offset = t.lastOutTS - inTS
	return t.lastOutTS
}

// Rewrite computes the adjusted timestamp for inTS without modifying the internal state
func (t *TimestampAdjuster) Rewrite(inTS uint32) uint32 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return inTS + t.offset
}

// Switch recalculates the timestamp offset when transitioning to a new incoming stream layer,
// advancing the output timestamp monotonically by frameDurationTS (e.g. 3000 ticks for 30fps at 90kHz)
// so the video decoder experiences uninterrupted playback without freezing.
func (t *TimestampAdjuster) Switch(newInTS uint32, frameDurationTS uint32) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if frameDurationTS == 0 {
		frameDurationTS = DefaultFrameDuration90kHz
	}

	if t.initialized {
		t.offset = (t.lastOutTS + frameDurationTS) - newInTS
	}
}

// GetOffset returns the current timestamp offset in a thread-safe manner
func (t *TimestampAdjuster) GetOffset() uint32 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.offset
}

// SetOffset explicitly sets the timestamp offset
func (t *TimestampAdjuster) SetOffset(offset uint32) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.offset = offset
}

// LastInTS returns the last observed input timestamp
func (t *TimestampAdjuster) LastInTS() uint32 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastInTS
}

// LastOutTS returns the last emitted output timestamp
func (t *TimestampAdjuster) LastOutTS() uint32 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastOutTS
}

// IsInitialized returns whether the adjuster has processed at least one packet
func (t *TimestampAdjuster) IsInitialized() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.initialized
}

// Reset clears the adjuster state
func (t *TimestampAdjuster) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.offset = 0
	t.lastInTS = 0
	t.lastOutTS = 0
	t.initialized = false
}
