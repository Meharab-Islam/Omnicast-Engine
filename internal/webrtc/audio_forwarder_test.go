package webrtc

import (
	"testing"
	"time"
)

func TestAudioForwarder_ShouldForward(t *testing.T) {
	detector := NewActiveSpeakerDetector()
	af := NewAudioForwarder(detector, "test-room")

	// Initially no speakers are active
	if af.ShouldForward("host") {
		t.Fatalf("expected ShouldForward to return false for unknown speaker")
	}

	// Force activate
	af.ForceActivate("host")
	if !af.ShouldForward("host") {
		t.Fatalf("expected ShouldForward to return true after ForceActivate")
	}

	// Force deactivate
	af.ForceDeactivate("host")
	if af.ShouldForward("host") {
		t.Fatalf("expected ShouldForward to return false after ForceDeactivate")
	}
}

func TestAudioForwarder_UpdateActiveSpeakerSet(t *testing.T) {
	detector := NewActiveSpeakerDetector()
	af := NewAudioForwarder(detector, "test-room")

	// Add speakers — some speaking, some silent
	detector.UpdateLevel("host", 5)       // Very loud (speaking)
	detector.UpdateLevel("cohost1", 15)   // Loud (speaking)
	detector.UpdateLevel("cohost2", 25)   // Moderate (speaking)
	detector.UpdateLevel("cohost3", 80)   // Quiet (not speaking)
	detector.UpdateLevel("cohost4", 127)  // Silent (not speaking)

	// Manually trigger update
	af.updateActiveSpeakerSet()

	// host, cohost1, cohost2 should be active (speaking); cohost3 and cohost4 should not
	if !af.ShouldForward("host") {
		t.Fatalf("expected host to be forwarded")
	}
	if !af.ShouldForward("cohost1") {
		t.Fatalf("expected cohost1 to be forwarded")
	}
	if !af.ShouldForward("cohost2") {
		t.Fatalf("expected cohost2 to be forwarded")
	}
	if af.ShouldForward("cohost3") {
		t.Fatalf("expected cohost3 to NOT be forwarded (quiet)")
	}
	if af.ShouldForward("cohost4") {
		t.Fatalf("expected cohost4 to NOT be forwarded (silent)")
	}
}

func TestAudioForwarder_GetActiveSpeakers(t *testing.T) {
	detector := NewActiveSpeakerDetector()
	af := NewAudioForwarder(detector, "test-room")

	af.ForceActivate("host")
	af.ForceActivate("cohost1")

	speakers := af.GetActiveSpeakers()
	if len(speakers) != 2 {
		t.Fatalf("expected 2 active speakers, got %d", len(speakers))
	}
	if !speakers["host"] || !speakers["cohost1"] {
		t.Fatalf("expected host and cohost1 in active speakers")
	}
}

func TestAudioForwarder_StartStop(t *testing.T) {
	detector := NewActiveSpeakerDetector()
	af := NewAudioForwarder(detector, "test-room")

	detector.UpdateLevel("host", 5)

	af.Start()
	time.Sleep(600 * time.Millisecond) // Wait for at least one tick
	af.Stop()

	// After one tick, host should be in the active set
	if !af.ShouldForward("host") {
		t.Fatalf("expected host to be forwarded after Start() tick")
	}
}

func TestAudioForwarder_FallbackWhenNoOneSpeaking(t *testing.T) {
	detector := NewActiveSpeakerDetector()
	af := NewAudioForwarder(detector, "test-room")

	// All speakers are silent (level > SpeakingThreshold)
	detector.UpdateLevel("host", 80)
	detector.UpdateLevel("cohost1", 100)

	af.updateActiveSpeakerSet()

	// Should still have at least 1 speaker (the loudest one as fallback)
	if af.GetActiveSpeakerCount() < 1 {
		t.Fatalf("expected at least 1 fallback speaker, got %d", af.GetActiveSpeakerCount())
	}
}
