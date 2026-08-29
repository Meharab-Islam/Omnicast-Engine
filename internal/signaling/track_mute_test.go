package signaling

import (
	"encoding/json"
	"testing"

	"omnicast/internal/models"
)

func TestTrackMuteSignaling(t *testing.T) {
	rm := NewRoomManager()
	hub := NewHub(rm)
	go hub.Run()

	room, err := rm.CreateRoom("mute-test-room", "host-alice")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	hostClient := &Client{
		ID:          "host-alice",
		RoomID:      room.RoomID,
		Role:        "host",
		Hub:         hub,
		RoomManager: rm,
		Send:        make(chan []byte, 50),
	}

	viewerClient := &Client{
		ID:          "viewer-bob",
		RoomID:      room.RoomID,
		Role:        "viewer",
		Hub:         hub,
		RoomManager: rm,
		Send:        make(chan []byte, 50),
	}

	_ = rm.JoinViewer(room.RoomID, viewerClient)

	// Test 1: Publisher mutes video track
	muteVideoPayload, _ := json.Marshal(map[string]any{
		"type":     "track_muted",
		"track_id": "host-alice_video",
		"muted":    true,
		"kind":     "video",
	})

	hostClient.handleMediaStateChange(&models.SignalingMessage{
		Event:   "track_muted",
		RoomID:  room.RoomID,
		Payload: muteVideoPayload,
	})

	// Verify state in room
	state, found := room.GetMediaState("host-alice")
	if !found {
		t.Fatalf("Expected media state for host-alice to exist")
	}
	if !state.MutedVideo {
		t.Errorf("Expected MutedVideo to be true")
	}
	if state.MutedAudio {
		t.Errorf("Expected MutedAudio to remain false")
	}

	// Test 2: Publisher mutes audio track
	muteAudioPayload, _ := json.Marshal(map[string]any{
		"type":     "track_muted",
		"track_id": "host-alice_audio",
		"muted":    true,
		"kind":     "audio",
	})

	hostClient.handleMediaStateChange(&models.SignalingMessage{
		Event:   "track_muted",
		RoomID:  room.RoomID,
		Payload: muteAudioPayload,
	})

	state, _ = room.GetMediaState("host-alice")
	if !state.MutedAudio || !state.MutedVideo {
		t.Errorf("Expected both MutedAudio and MutedVideo to be true, got audio=%v, video=%v", state.MutedAudio, state.MutedVideo)
	}

	// Test 3: Publisher un-mutes video track
	unmuteVideoPayload, _ := json.Marshal(map[string]any{
		"type":     "track_muted",
		"track_id": "host-alice_video",
		"muted":    false,
		"kind":     "video",
	})

	hostClient.handleMediaStateChange(&models.SignalingMessage{
		Event:   "track_unmuted",
		RoomID:  room.RoomID,
		Payload: unmuteVideoPayload,
	})

	state, _ = room.GetMediaState("host-alice")
	if state.MutedVideo {
		t.Errorf("Expected MutedVideo to be false after un-mute")
	}
	if !state.MutedAudio {
		t.Errorf("Expected MutedAudio to remain true")
	}
}
