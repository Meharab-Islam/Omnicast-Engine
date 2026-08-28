
package webrtc

import (
	"log"
	"sync"
	"time"
)

// DefaultForwardedSpeakers is the default number of top speakers whose audio is forwarded to viewers.
const DefaultForwardedSpeakers = 4

// DefaultSpeakerUpdateInterval is how often the AudioForwarder re-evaluates the active speaker set.
const DefaultSpeakerUpdateInterval = 500 * time.Millisecond

// AudioForwarder implements selective audio forwarding for the SFU.
// It holds a reference to the ActiveSpeakerDetector and maintains an activeSpeakerSet.
// Every 500ms, it queries GetTopSpeakers(maxForwarded) and updates the set.
// Only audio RTP packets from speakers in the active set are forwarded to Viewers —
// all other co-host audio is muted at the server level (packets silently dropped).
type AudioForwarder struct {
	mu             sync.RWMutex
	detector       *ActiveSpeakerDetector
	activeSpeakers map[string]bool // currently forwarded speaker IDs
	maxForwarded   int
	roomID         string
	stopCh         chan struct{}
	stopped        bool
}

// NewAudioForwarder creates a new AudioForwarder tied to an ActiveSpeakerDetector.
func NewAudioForwarder(detector *ActiveSpeakerDetector, roomID string) *AudioForwarder {
	return &AudioForwarder{
		detector:       detector,
		activeSpeakers: make(map[string]bool),
		maxForwarded:   DefaultForwardedSpeakers,
		roomID:         roomID,
		stopCh:         make(chan struct{}),
	}
}

// NewAudioForwarderWithConfig creates a new AudioForwarder with custom max forwarded speakers.
func NewAudioForwarderWithConfig(detector *ActiveSpeakerDetector, roomID string, maxForwarded int) *AudioForwarder {
	if maxForwarded <= 0 {
		maxForwarded = DefaultForwardedSpeakers
	}
	return &AudioForwarder{
		detector:       detector,
		activeSpeakers: make(map[string]bool),
		maxForwarded:   maxForwarded,
		roomID:         roomID,
		stopCh:         make(chan struct{}),
	}
}

// ShouldForward returns true if the given speakerID is in the active speaker set
// and their audio should be forwarded to viewers.
func (af *AudioForwarder) ShouldForward(speakerID string) bool {
	af.mu.RLock()
	defer af.mu.RUnlock()
	return af.activeSpeakers[speakerID]
}

// GetActiveSpeakers returns a copy of the current active speaker set.
func (af *AudioForwarder) GetActiveSpeakers() map[string]bool {
	af.mu.RLock()
	defer af.mu.RUnlock()
	result := make(map[string]bool, len(af.activeSpeakers))
	for k, v := range af.activeSpeakers {
		result[k] = v
	}
	return result
}

// GetActiveSpeakerCount returns the number of currently forwarded speakers.
func (af *AudioForwarder) GetActiveSpeakerCount() int {
	af.mu.RLock()
	defer af.mu.RUnlock()
	return len(af.activeSpeakers)
}

// updateActiveSpeakerSet queries the detector for top speakers and updates the active set.
// Logs speaker transitions when the set changes.
func (af *AudioForwarder) updateActiveSpeakerSet() {
	if af.detector == nil {
		return
	}

	topSpeakers := af.detector.GetTopSpeakers(af.maxForwarded)

	newSet := make(map[string]bool, len(topSpeakers))
	for _, speaker := range topSpeakers {
		if speaker.IsSpeaking {
			newSet[speaker.SpeakerID] = true
		}
	}

	// If no one is speaking, keep the host/main speaker in the set
	// to avoid complete audio silence
	if len(newSet) == 0 && len(topSpeakers) > 0 {
		newSet[topSpeakers[0].SpeakerID] = true
	}

	af.mu.Lock()
	defer af.mu.Unlock()

	// Log speaker transitions
	for speakerID := range newSet {
		if !af.activeSpeakers[speakerID] {
			log.Printf("[Active Speaker] Room %s: Speaker '%s' promoted to active (audio forwarding enabled)\n",
				af.roomID, speakerID)
		}
	}
	for speakerID := range af.activeSpeakers {
		if !newSet[speakerID] {
			log.Printf("[Active Speaker] Room %s: Speaker '%s' demoted (audio muted at server level)\n",
				af.roomID, speakerID)
		}
	}

	af.activeSpeakers = newSet
}

// ForceActivate adds a speaker to the active set immediately (e.g., the host should always be forwarded).
func (af *AudioForwarder) ForceActivate(speakerID string) {
	af.mu.Lock()
	defer af.mu.Unlock()
	af.activeSpeakers[speakerID] = true
}

// ForceDeactivate removes a speaker from the active set immediately.
func (af *AudioForwarder) ForceDeactivate(speakerID string) {
	af.mu.Lock()
	defer af.mu.Unlock()
	delete(af.activeSpeakers, speakerID)
}

// Start launches the background goroutine that re-evaluates the active speaker set every 500ms.
func (af *AudioForwarder) Start() {
	go func() {
		ticker := time.NewTicker(DefaultSpeakerUpdateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-af.stopCh:
				return
			case <-ticker.C:
				af.updateActiveSpeakerSet()
			}
		}
	}()
}

// Stop terminates the background goroutine.
func (af *AudioForwarder) Stop() {
	af.mu.Lock()
	defer af.mu.Unlock()
	if !af.stopped {
		af.stopped = true
		close(af.stopCh)
	}
}
