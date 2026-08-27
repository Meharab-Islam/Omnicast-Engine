package models

import (
	"testing"

	"github.com/pion/webrtc/v3"
)

func TestRoomCoHostTracks(t *testing.T) {
	room := NewRoom("test-room", "host-123")

	if room.HostScore != 0 {
		t.Fatalf("expected host score 0, got %d", room.HostScore)
	}

	newScore := room.AddHostScore(50)
	if newScore != 50 || room.GetHostScore() != 50 {
		t.Fatalf("expected host score 50, got %d", newScore)
	}

	// Create dummy track
	capability := webrtc.RTPCodecCapability{
		MimeType:  webrtc.MimeTypeVP8,
		ClockRate: 90000,
	}
	track, err := webrtc.NewTrackLocalStaticRTP(capability, "video", "pion")
	if err != nil {
		t.Fatalf("failed to create track: %v", err)
	}

	// Test CoHost track registration
	coHostID := "cohost-456"
	room.SetCoHostTrack(coHostID, track)

	fetchedTrack, found := room.GetCoHostTrack(coHostID)
	if !found || fetchedTrack == nil {
		t.Fatalf("expected to find cohost track for %s", coHostID)
	}

	allTracks := room.GetAllCoHostTracks()
	if len(allTracks) != 1 {
		t.Fatalf("expected 1 cohost track, got %d", len(allTracks))
	}

	room.RemoveCoHostTrack(coHostID)
	_, foundAfterDelete := room.GetCoHostTrack(coHostID)
	if foundAfterDelete {
		t.Fatalf("expected cohost track to be deleted")
	}
}

func TestRoomSimulcastTracks(t *testing.T) {
	room := NewRoom("simulcast-room", "host-sim")

	capability := webrtc.RTPCodecCapability{
		MimeType:  webrtc.MimeTypeVP8,
		ClockRate: 90000,
	}

	trackQ, _ := webrtc.NewTrackLocalStaticRTP(capability, "video-q", "pion")
	trackH, _ := webrtc.NewTrackLocalStaticRTP(capability, "video-h", "pion")
	trackF, _ := webrtc.NewTrackLocalStaticRTP(capability, "video-f", "pion")

	room.SetVideoTrackRID("q", trackQ)
	room.SetVideoTrackRID("h", trackH)
	room.SetVideoTrackRID("f", trackF)

	if room.GetVideoTrackByRID("q") != trackQ {
		t.Fatalf("expected trackQ for RID 'q'")
	}
	if room.GetVideoTrackByRID("h") != trackH {
		t.Fatalf("expected trackH for RID 'h'")
	}
	if room.GetVideoTrackByRID("f") != trackF {
		t.Fatalf("expected trackF for RID 'f'")
	}

	// Basic adaptive default should return Medium 'h'
	defaultTrack := room.GetDefaultViewerVideoTrack()
	if defaultTrack != trackH {
		t.Fatalf("expected default viewer video track to be Medium ('h')")
	}

	allVideoTracks := room.GetAllVideoTracks()
	if len(allVideoTracks) != 3 {
		t.Fatalf("expected 3 simulcast layers, got %d", len(allVideoTracks))
	}
}

func TestRoomState(t *testing.T) {
	room := NewRoomWithName("state-room", "Studio A", "host-xyz")
	room.SetHostScore(120)
	room.SetActiveSeat("1", "cohost-1")
	room.SetMediaState("host-xyz", MediaState{MutedAudio: false, MutedVideo: false})
	room.SetMediaState("cohost-1", MediaState{MutedAudio: true, MutedVideo: false})

	state := room.GetRoomState()
	if state == nil {
		t.Fatal("expected non-nil RoomState")
	}
	if state.RoomID != "state-room" || state.HostID != "host-xyz" {
		t.Fatalf("unexpected room/host ID: %+v", state)
	}
	if state.HostScore != 120 {
		t.Fatalf("expected host score 120, got %d", state.HostScore)
	}
	if state.ActiveSeats["1"] != "cohost-1" {
		t.Fatalf("expected active seat 1 to be cohost-1, got %s", state.ActiveSeats["1"])
	}
	if !state.MediaStates["cohost-1"].MutedAudio {
		t.Fatal("expected cohost-1 audio to be muted")
	}

	room.RemoveActiveSeat("1")
	stateAfter := room.GetRoomState()
	if _, exists := stateAfter.ActiveSeats["1"]; exists {
		t.Fatal("expected seat 1 to be removed")
	}
}

