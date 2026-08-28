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

	// Basic adaptive default should return Full HD 'f'
	defaultTrack := room.GetDefaultViewerVideoTrack()
	if defaultTrack != trackF {
		t.Fatalf("expected default viewer video track to be Full HD ('f')")
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

func TestRoomTypeAndStateSync(t *testing.T) {
	roomAudio := NewRoom("audio-room-1", "host-aud")
	roomAudio.SetRoomType("audio")
	if roomAudio.GetRoomType() != "audio" {
		t.Fatalf("expected room type 'audio', got '%s'", roomAudio.GetRoomType())
	}
	stateAudio := roomAudio.GetRoomState()
	if stateAudio.RoomType != "audio" {
		t.Fatalf("expected state RoomType 'audio', got '%s'", stateAudio.RoomType)
	}

	roomVideo := NewRoom("video-room-1", "host-vid")
	if roomVideo.GetRoomType() != "video" {
		t.Fatalf("expected default room type 'video', got '%s'", roomVideo.GetRoomType())
	}
}

func TestRoomParticipantsAndPresence(t *testing.T) {
	room := NewRoom("presence-test-room", "host-p1")

	p1 := &Participant{
		UserID:      "user-1",
		DisplayName: "Bob Ross",
		AvatarURL:   "https://img.com/bob.jpg",
		Role:        "viewer",
		Metadata: map[string]interface{}{
			"level":     42,
			"badge":     "VIP_GOLD",
			"vip_frame": "dragon_frame.svg",
		},
	}
	p2 := &Participant{
		UserID:      "user-2",
		DisplayName: "Alice",
		AvatarURL:   "https://img.com/alice.jpg",
		Role:        "cohost",
		Metadata: map[string]interface{}{
			"level": 99,
		},
	}

	room.AddParticipant(p1)
	room.AddParticipant(p2)

	fetched, ok := room.GetParticipant("user-1")
	if !ok || fetched.DisplayName != "Bob Ross" {
		t.Fatalf("expected to find participant Bob Ross, got %+v", fetched)
	}
	if fetched.Metadata["badge"] != "VIP_GOLD" {
		t.Fatalf("expected VIP_GOLD badge, got %v", fetched.Metadata["badge"])
	}

	list := room.GetParticipantsList()
	if len(list) != 2 {
		t.Fatalf("expected 2 participants in list, got %d", len(list))
	}

	// Test RoomState includes participants
	state := room.GetRoomState()
	if len(state.Participants) != 2 {
		t.Fatalf("expected 2 participants in RoomState, got %d", len(state.Participants))
	}

	// Test Presence Batching
	room.EnqueuePresenceJoin(p1)
	room.EnqueuePresenceLeave("user-old")

	joins, leaves, totalCount, pList := room.FlushPresence()
	if len(joins) != 1 || joins[0].UserID != "user-1" {
		t.Fatalf("expected 1 join for user-1, got %v", joins)
	}
	if len(leaves) != 1 || leaves[0] != "user-old" {
		t.Fatalf("expected 1 leave for user-old, got %v", leaves)
	}
	if totalCount != 0 {
		t.Fatalf("expected 0 viewers count, got %d", totalCount)
	}
	if len(pList) != 2 {
		t.Fatalf("expected 2 participants in snapshot, got %d", len(pList))
	}

	// Second flush should be empty
	j2, l2, _, _ := room.FlushPresence()
	if len(j2) != 0 || len(l2) != 0 {
		t.Fatal("expected empty queue after flush")
	}

	// Remove participant
	room.RemoveParticipant("user-1")
	_, found := room.GetParticipant("user-1")
	if found {
		t.Fatal("expected participant user-1 to be deleted")
	}
}

func TestRoomTrackSwitcherRegistry(t *testing.T) {
	room := NewRoom("switcher-test-room", "host-s1")

	dummySwitcher := "switcher-instance-123"
	room.RegisterTrackSwitcher("viewer-101", dummySwitcher)

	s, ok := room.GetTrackSwitcher("viewer-101")
	if !ok || s != dummySwitcher {
		t.Fatalf("expected to retrieve dummy switcher, got %v", s)
	}

	all := room.GetAllTrackSwitchers()
	if len(all) != 1 {
		t.Fatalf("expected 1 switcher, got %d", len(all))
	}

	room.UnregisterTrackSwitcher("viewer-101")
	_, okAfter := room.GetTrackSwitcher("viewer-101")
	if okAfter {
		t.Fatal("expected switcher to be unregistered")
	}
}

