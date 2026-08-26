package models

import (
	"encoding/json"
	"errors"
)

// Common signaling event types
const (
	EventJoin   = "join"
	EventOffer  = "offer"
	EventAnswer = "answer"
	EventICE    = "ice"
	EventLeave  = "leave"
)

// SignalingMessage represents the message protocol format for WebRTC signaling
type SignalingMessage struct {
	Event        string          `json:"event"`
	RoomID       string          `json:"room_id,omitempty"`
	RoomName     string          `json:"room_name,omitempty"`
	UserID       string          `json:"user_id,omitempty"`
	TargetUser   string          `json:"target_user,omitempty"`
	TotalViewers int             `json:"total_viewers,omitempty"`
	ViewersList  []string        `json:"viewers_list,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
}

// ParseSignalingMessage parses raw byte data into a SignalingMessage struct
func ParseSignalingMessage(data []byte) (*SignalingMessage, error) {
	if len(data) == 0 {
		return nil, errors.New("empty message data")
	}

	var msg SignalingMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}

	if msg.Event == "" {
		return nil, errors.New("invalid signaling message: 'event' field is required")
	}

	return &msg, nil
}

// Encode converts the SignalingMessage into JSON bytes
func (m *SignalingMessage) Encode() ([]byte, error) {
	return json.Marshal(m)
}
