package models

import "time"

// MediaState tracks audio and video mute/unmute states for a user in a room
type MediaState struct {
	MutedAudio bool `json:"muted_audio"`
	MutedVideo bool `json:"muted_video"`
}

// RoomState represents the standardized serialized state of a room in Redis
type RoomState struct {
	RoomID       string                `json:"room_id"`
	RoomName     string                `json:"room_name,omitempty"`
	HostID       string                `json:"host_id"`
	TotalViewers int                   `json:"total_viewers"`
	HostScore    int64                 `json:"host_score"`
	ActiveSeats  map[string]string     `json:"active_seats"`
	MediaStates  map[string]MediaState `json:"media_states"`
	CreatedAt    time.Time             `json:"created_at"`
}
