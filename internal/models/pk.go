package models

import "time"

// PKSession represents an active cross-room PK battle between two rooms
type PKSession struct {
	SessionID string    `json:"session_id"`
	RoomID1   string    `json:"room_id_1"`
	RoomID2   string    `json:"room_id_2"`
	HostID1   string    `json:"host_id_1"`
	HostID2   string    `json:"host_id_2"`
	Score1    int64     `json:"score_1"`
	Score2    int64     `json:"score_2"`
	Status    string    `json:"status"` // active, ended
	CreatedAt time.Time `json:"created_at"`
}
