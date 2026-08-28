package webrtc

import (
	"sort"
	"sync"
	"time"
)

// SpeakingThreshold is the audio level below which a speaker is considered "active" (speaking).
// Per RFC 6464: 0 = loudest, 127 = silence. A threshold of 30 captures normal speech.
const SpeakingThreshold uint8 = 30

// DefaultMaxSpeakers is the default number of top speakers to track and forward.
const DefaultMaxSpeakers = 4

// DefaultSmoothingWindow is the rolling average window for audio level smoothing (~300ms).
const DefaultSmoothingWindow = 300 * time.Millisecond

// SpeakerInfo represents the current audio state of a single speaker.
type SpeakerInfo struct {
	SpeakerID  string
	AudioLevel uint8 // 0 = loudest, 127 = silence (RFC 6464)
	IsSpeaking bool  // true if level < SpeakingThreshold
}

// speakerState tracks the internal rolling audio level state for a single speaker.
type speakerState struct {
	speakerID    string
	lastLevel    uint8
	avgLevel     float64
	sampleCount  int
	lastUpdate   time.Time
	isSpeaking   bool
}

// ActiveSpeakerDetector parses the urn:ietf:params:rtp-hdrext:ssrc-audio-level
// RTP header extension and maintains a map of speakerID → audioLevel with a rolling
// average over a configurable smoothing window. Thread-safe via sync.RWMutex.
type ActiveSpeakerDetector struct {
	mu              sync.RWMutex
	speakers        map[string]*speakerState
	maxSpeakers     int
	smoothingWindow time.Duration
}

// NewActiveSpeakerDetector creates a new ActiveSpeakerDetector with default configuration.
func NewActiveSpeakerDetector() *ActiveSpeakerDetector {
	return &ActiveSpeakerDetector{
		speakers:        make(map[string]*speakerState),
		maxSpeakers:     DefaultMaxSpeakers,
		smoothingWindow: DefaultSmoothingWindow,
	}
}

// NewActiveSpeakerDetectorWithConfig creates a new ActiveSpeakerDetector with custom configuration.
func NewActiveSpeakerDetectorWithConfig(maxSpeakers int, smoothingWindow time.Duration) *ActiveSpeakerDetector {
	if maxSpeakers <= 0 {
		maxSpeakers = DefaultMaxSpeakers
	}
	if smoothingWindow <= 0 {
		smoothingWindow = DefaultSmoothingWindow
	}
	return &ActiveSpeakerDetector{
		speakers:        make(map[string]*speakerState),
		maxSpeakers:     maxSpeakers,
		smoothingWindow: smoothingWindow,
	}
}

// ParseAudioLevel extracts the audio level from the ssrc-audio-level RTP header extension payload.
// Per RFC 6464 Section 3:
//
//	 0                   1
//	 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5
//	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//	|V|   level     |
//	+-+-+-+-+-+-+-+-+
//
// V (bit 0): Voice Activity flag (1 = voice detected)
// level (bits 1-7): Audio level in -dBov (0 = loudest, 127 = silence)
//
// Returns (level uint8, voice bool).
func ParseAudioLevel(extensionPayload []byte) (level uint8, voice bool) {
	if len(extensionPayload) == 0 {
		return 127, false // silence
	}
	voice = (extensionPayload[0] & 0x80) != 0
	level = extensionPayload[0] & 0x7F
	return level, voice
}

// UpdateLevel updates the audio level for a given speaker.
// It maintains a rolling exponential moving average over the smoothing window.
func (d *ActiveSpeakerDetector) UpdateLevel(speakerID string, level uint8) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	state, exists := d.speakers[speakerID]
	if !exists {
		state = &speakerState{
			speakerID:   speakerID,
			lastLevel:   level,
			avgLevel:    float64(level),
			sampleCount: 1,
			lastUpdate:  now,
			isSpeaking:  level < SpeakingThreshold,
		}
		d.speakers[speakerID] = state
		return
	}

	// Exponential Moving Average (EMA) with alpha = 0.3 for smooth transitions
	const alpha = 0.3
	state.avgLevel = alpha*float64(level) + (1-alpha)*state.avgLevel
	state.lastLevel = level
	state.sampleCount++
	state.lastUpdate = now
	state.isSpeaking = uint8(state.avgLevel) < SpeakingThreshold
}

// RemoveSpeaker removes a speaker from the detector (e.g., when a co-host disconnects).
func (d *ActiveSpeakerDetector) RemoveSpeaker(speakerID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.speakers, speakerID)
}

// GetTopSpeakers returns the top N loudest active speakers sorted by audio level (lowest = loudest).
// Stale speakers (not updated within 2× smoothingWindow) are treated as silent.
func (d *ActiveSpeakerDetector) GetTopSpeakers(n int) []SpeakerInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if n <= 0 {
		n = d.maxSpeakers
	}

	now := time.Now()
	staleThreshold := 2 * d.smoothingWindow

	var activeSpeakers []SpeakerInfo
	for _, state := range d.speakers {
		effectiveLevel := uint8(state.avgLevel)

		// Treat stale speakers as silent
		if now.Sub(state.lastUpdate) > staleThreshold {
			effectiveLevel = 127 // silence
		}

		activeSpeakers = append(activeSpeakers, SpeakerInfo{
			SpeakerID:  state.speakerID,
			AudioLevel: effectiveLevel,
			IsSpeaking: effectiveLevel < SpeakingThreshold,
		})
	}

	// Sort by audio level ascending (0 = loudest first)
	sort.Slice(activeSpeakers, func(i, j int) bool {
		return activeSpeakers[i].AudioLevel < activeSpeakers[j].AudioLevel
	})

	if len(activeSpeakers) > n {
		activeSpeakers = activeSpeakers[:n]
	}

	return activeSpeakers
}

// GetSpeakerLevel returns the current average audio level for a specific speaker.
func (d *ActiveSpeakerDetector) GetSpeakerLevel(speakerID string) (uint8, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	state, exists := d.speakers[speakerID]
	if !exists {
		return 127, false
	}
	return uint8(state.avgLevel), state.isSpeaking
}

// GetSpeakerCount returns the total number of tracked speakers.
func (d *ActiveSpeakerDetector) GetSpeakerCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.speakers)
}
