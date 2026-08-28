package models

import "time"

// MediaState tracks audio and video mute/unmute states for a user in a room
type MediaState struct {
	MutedAudio bool `json:"muted_audio"`
	MutedVideo bool `json:"muted_video"`
}

// PKState represents the real-time PK battle state of a room
type PKState struct {
	IsActive       bool   `json:"is_active"`
	SessionID      string `json:"session_id,omitempty"`
	OpponentID     string `json:"opponent_id,omitempty"`
	OpponentRoomID string `json:"opponent_room_id,omitempty"`
	HostScore      int64  `json:"host_score"`
	OpponentScore  int64  `json:"opponent_score"`
}

// RoomState represents the standardized serialized state of a room in Redis
type RoomState struct {
	RoomID       string                `json:"room_id"`
	RoomName     string                `json:"room_name,omitempty"`
	RoomType     string                `json:"room_type,omitempty"` // "video" or "audio"
	HostID       string                `json:"host_id"`
	TotalViewers int                   `json:"total_viewers"`
	HostScore    int64                 `json:"host_score"`
	ActiveSeats  map[string]string     `json:"active_seats"`
	MediaStates  map[string]MediaState `json:"media_states"`
	Participants []*Participant        `json:"participants,omitempty"`
	PKState      *PKState              `json:"pk_state,omitempty"`
	CreatedAt    time.Time             `json:"created_at"`
}
