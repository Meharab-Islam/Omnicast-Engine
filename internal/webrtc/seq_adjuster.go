package webrtc

import (
	"sync"
)

// SequenceNumberAdjuster maintains an offset variable to rewrite RTP sequence numbers continuously
// across simulcast layer switches, stream transitions, or track replacements, preventing sequence gaps or jumps.
type SequenceNumberAdjuster struct {
	mu          sync.RWMutex
	offset      uint16 // Offset added to incoming sequence numbers: outSeq = inSeq + offset
	lastInSeq   uint16 // Last observed incoming sequence number
	lastOutSeq  uint16 // Last emitted rewritten sequence number
	initialized bool   // Indicates whether at least one packet has been processed
}

// NewSequenceNumberAdjuster creates and initializes a new SequenceNumberAdjuster
func NewSequenceNumberAdjuster() *SequenceNumberAdjuster {
	return &SequenceNumberAdjuster{
		offset: 0,
	}
}

// Adjust rewrites an incoming RTP sequence number using the current offset:
// outSeq = inSeq + offset
// It updates the internal lastInSeq and lastOutSeq states in a thread-safe manner.
func (s *SequenceNumberAdjuster) Adjust(inSeq uint16) uint16 {
	s.mu.Lock()
	defer s.mu.Unlock()

	outSeq := inSeq + s.offset
	s.lastInSeq = inSeq
	s.lastOutSeq = outSeq
	s.initialized = true
	return outSeq
}

// Rewrite computes the adjusted sequence number for inSeq without updating internal state
func (s *SequenceNumberAdjuster) Rewrite(inSeq uint16) uint16 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return inSeq + s.offset
}

// Switch recalculates the offset when transitioning to a new incoming sequence space (e.g. from a different simulcast layer)
// such that the next output sequence number will be strictly sequential (lastOutSeq + 1).
func (s *SequenceNumberAdjuster) Switch(newInSeq uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.initialized {
		s.offset = (s.lastOutSeq + 1) - newInSeq
	}
}

// GetOffset returns the current sequence number offset in a thread-safe manner
func (s *SequenceNumberAdjuster) GetOffset() uint16 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.offset
}

// SetOffset explicitly sets the sequence number offset
func (s *SequenceNumberAdjuster) SetOffset(offset uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.offset = offset
}

// LastInSeq returns the last observed input sequence number
func (s *SequenceNumberAdjuster) LastInSeq() uint16 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastInSeq
}

// LastOutSeq returns the last emitted output sequence number
func (s *SequenceNumberAdjuster) LastOutSeq() uint16 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastOutSeq
}

// IsInitialized returns whether the adjuster has processed at least one packet
func (s *SequenceNumberAdjuster) IsInitialized() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.initialized
}

// Reset clears the adjuster state
func (s *SequenceNumberAdjuster) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.offset = 0
	s.lastInSeq = 0
	s.lastOutSeq = 0
	s.initialized = false
}
