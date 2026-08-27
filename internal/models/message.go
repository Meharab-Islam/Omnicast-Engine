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

// WSMessage defines the standard base WebSocket message envelope for the SDK
type WSMessage struct {
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// SignalingMessage represents the message protocol format for WebRTC signaling and real-time events
type SignalingMessage struct {
	Action       string          `json:"action,omitempty"`
	Event        string          `json:"event"`
	RoomID       string          `json:"room_id,omitempty"`
	RoomName     string          `json:"room_name,omitempty"`
	UserID       string          `json:"user_id,omitempty"`
	TargetUser   string          `json:"target_user,omitempty"`
	TotalViewers int             `json:"total_viewers,omitempty"`
	ViewersList  []string        `json:"viewers_list,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
}

// ParseSignalingMessage parses raw byte data into a SignalingMessage struct supporting both action and event
func ParseSignalingMessage(data []byte) (*SignalingMessage, error) {
	if len(data) == 0 {
		return nil, errors.New("empty message data")
	}

	var raw struct {
		Action       string          `json:"action"`
		Event        string          `json:"event"`
		RoomID       string          `json:"room_id"`
		RoomName     string          `json:"room_name"`
		UserID       string          `json:"user_id"`
		TargetUser   string          `json:"target_user"`
		TotalViewers int             `json:"total_viewers"`
		ViewersList  []string        `json:"viewers_list"`
		Payload      json.RawMessage `json:"payload"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	evt := raw.Event
	if evt == "" {
		evt = raw.Action
	}

	if evt == "" {
		return nil, errors.New("invalid signaling message: 'action' or 'event' field is required")
	}

	return &SignalingMessage{
		Action:       evt,
		Event:        evt,
		RoomID:       raw.RoomID,
		RoomName:     raw.RoomName,
		UserID:       raw.UserID,
		TargetUser:   raw.TargetUser,
		TotalViewers: raw.TotalViewers,
		ViewersList:  raw.ViewersList,
		Payload:      raw.Payload,
	}, nil
}

// Encode converts the SignalingMessage into JSON bytes
func (m *SignalingMessage) Encode() ([]byte, error) {
	if m.Action == "" && m.Event != "" {
		m.Action = m.Event
	}
	return json.Marshal(m)
}
