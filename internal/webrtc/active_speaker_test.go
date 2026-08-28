package webrtc

import (
	"testing"
	"time"
)

func TestParseAudioLevel(t *testing.T) {
	// Empty payload -> silence
	level, voice := ParseAudioLevel(nil)
	if level != 127 || voice {
		t.Fatalf("expected (127, false) for nil payload, got (%d, %v)", level, voice)
	}
	level, voice = ParseAudioLevel([]byte{})
	if level != 127 || voice {
		t.Fatalf("expected (127, false) for empty payload, got (%d, %v)", level, voice)
	}

	// Voice active, level 10 (loud): 0x80 | 0x0A = 0x8A
	level, voice = ParseAudioLevel([]byte{0x8A})
	if level != 10 || !voice {
		t.Fatalf("expected (10, true), got (%d, %v)", level, voice)
	}

	// Voice inactive, level 50 (moderate): 0x32
	level, voice = ParseAudioLevel([]byte{0x32})
	if level != 50 || voice {
		t.Fatalf("expected (50, false), got (%d, %v)", level, voice)
	}

	// Voice active, level 0 (loudest): 0x80
	level, voice = ParseAudioLevel([]byte{0x80})
	if level != 0 || !voice {
		t.Fatalf("expected (0, true), got (%d, %v)", level, voice)
	}

	// Voice inactive, level 127 (silence): 0x7F
	level, voice = ParseAudioLevel([]byte{0x7F})
	if level != 127 || voice {
		t.Fatalf("expected (127, false), got (%d, %v)", level, voice)
	}
}

func TestActiveSpeakerDetector_UpdateAndGet(t *testing.T) {
	detector := NewActiveSpeakerDetector()

	// Initially empty
	if detector.GetSpeakerCount() != 0 {
		t.Fatalf("expected 0 speakers, got %d", detector.GetSpeakerCount())
	}

	// Add speakers with different levels
	detector.UpdateLevel("host", 5)   // Very loud
	detector.UpdateLevel("cohost1", 20) // Loud
	detector.UpdateLevel("cohost2", 60) // Quiet
	detector.UpdateLevel("cohost3", 127) // Silent

	if detector.GetSpeakerCount() != 4 {
		t.Fatalf("expected 4 speakers, got %d", detector.GetSpeakerCount())
	}

	// GetSpeakerLevel
	level, isSpeaking := detector.GetSpeakerLevel("host")
	if level != 5 || !isSpeaking {
		t.Fatalf("expected host level=5, speaking=true, got level=%d, speaking=%v", level, isSpeaking)
	}

	level, isSpeaking = detector.GetSpeakerLevel("cohost3")
	if isSpeaking {
		t.Fatalf("expected cohost3 to not be speaking at level 127")
	}

	// Nonexistent speaker
	level, isSpeaking = detector.GetSpeakerLevel("nonexistent")
	if level != 127 || isSpeaking {
		t.Fatalf("expected (127, false) for nonexistent speaker, got (%d, %v)", level, isSpeaking)
	}
}

func TestActiveSpeakerDetector_GetTopSpeakers(t *testing.T) {
	detector := NewActiveSpeakerDetector()

	// Add 5 speakers
	detector.UpdateLevel("host", 5)
	detector.UpdateLevel("cohost1", 15)
	detector.UpdateLevel("cohost2", 25)
	detector.UpdateLevel("cohost3", 50)
	detector.UpdateLevel("cohost4", 80)

	// Get top 3
	top := detector.GetTopSpeakers(3)
	if len(top) != 3 {
		t.Fatalf("expected 3 top speakers, got %d", len(top))
	}

	// Should be sorted by loudest first
	if top[0].SpeakerID != "host" {
		t.Fatalf("expected top[0] to be 'host', got '%s'", top[0].SpeakerID)
	}
	if top[1].SpeakerID != "cohost1" {
		t.Fatalf("expected top[1] to be 'cohost1', got '%s'", top[1].SpeakerID)
	}
	if top[2].SpeakerID != "cohost2" {
		t.Fatalf("expected top[2] to be 'cohost2', got '%s'", top[2].SpeakerID)
	}

	// Get top 4 (default)
	top4 := detector.GetTopSpeakers(0)
	if len(top4) != 4 {
		t.Fatalf("expected 4 top speakers (default), got %d", len(top4))
	}
}

func TestActiveSpeakerDetector_RemoveSpeaker(t *testing.T) {
	detector := NewActiveSpeakerDetector()

	detector.UpdateLevel("host", 5)
	detector.UpdateLevel("cohost1", 15)

	if detector.GetSpeakerCount() != 2 {
		t.Fatalf("expected 2 speakers, got %d", detector.GetSpeakerCount())
	}

	detector.RemoveSpeaker("cohost1")

	if detector.GetSpeakerCount() != 1 {
		t.Fatalf("expected 1 speaker after removal, got %d", detector.GetSpeakerCount())
	}
}

func TestActiveSpeakerDetector_EMASmoothing(t *testing.T) {
	detector := NewActiveSpeakerDetector()

	// Feed a burst of loud samples followed by silent samples
	for i := 0; i < 10; i++ {
		detector.UpdateLevel("speaker", 5) // loud
	}
	level, _ := detector.GetSpeakerLevel("speaker")
	if level > 10 {
		t.Fatalf("expected smoothed level to be near 5 after loud burst, got %d", level)
	}

	// Feed silent samples — EMA should gradually increase
	for i := 0; i < 20; i++ {
		detector.UpdateLevel("speaker", 127)
	}
	level, _ = detector.GetSpeakerLevel("speaker")
	if level < 50 {
		t.Fatalf("expected smoothed level to rise after silent samples, got %d", level)
	}
}

func TestActiveSpeakerDetector_StaleSpeaker(t *testing.T) {
	detector := NewActiveSpeakerDetectorWithConfig(4, 50*time.Millisecond)

	detector.UpdateLevel("host", 5)
	detector.UpdateLevel("stale", 10)

	// Wait for stale to expire
	time.Sleep(120 * time.Millisecond)

	// Update host to keep it fresh
	detector.UpdateLevel("host", 5)

	top := detector.GetTopSpeakers(2)
	if len(top) != 2 {
		t.Fatalf("expected 2 speakers, got %d", len(top))
	}

	// Host should be first (fresh and loud), stale should be treated as silent
	if top[0].SpeakerID != "host" {
		t.Fatalf("expected top[0] to be 'host', got '%s'", top[0].SpeakerID)
	}
	if top[1].AudioLevel != 127 {
		t.Fatalf("expected stale speaker to have level 127 (silent), got %d", top[1].AudioLevel)
	}
}
