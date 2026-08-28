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
	Action       string                 `json:"action,omitempty"`
	Event        string                 `json:"event"`
	RoomID       string                 `json:"room_id,omitempty"`
	RoomName     string                 `json:"room_name,omitempty"`
	RoomType     string                 `json:"room_type,omitempty"` // "video" or "audio"
	UserID       string                 `json:"user_id,omitempty"`
	DisplayName  string                 `json:"display_name,omitempty"`
	AvatarURL    string                 `json:"avatar_url,omitempty"`
	Role         string                 `json:"role,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	TargetUser   string                 `json:"target_user,omitempty"`
	TotalViewers int                    `json:"total_viewers,omitempty"`
	ViewersList  []string               `json:"viewers_list,omitempty"`
	Payload      json.RawMessage        `json:"payload,omitempty"`
}

// ParseSignalingMessage parses raw byte data into a SignalingMessage struct supporting both action and event
func ParseSignalingMessage(data []byte) (*SignalingMessage, error) {
	if len(data) == 0 {
		return nil, errors.New("empty message data")
	}

	var raw struct {
		Action       string                 `json:"action"`
		Event        string                 `json:"event"`
		RoomID       string                 `json:"room_id"`
		RoomName     string                 `json:"room_name"`
		RoomType     string                 `json:"room_type"`
		UserID       string                 `json:"user_id"`
		DisplayName  string                 `json:"display_name"`
		UserName     string                 `json:"user_name"`
		AvatarURL    string                 `json:"avatar_url"`
		Role         string                 `json:"role"`
		Metadata     map[string]interface{} `json:"metadata"`
		TargetUser   string                 `json:"target_user"`
		TotalViewers int                    `json:"total_viewers"`
		ViewersList  []string               `json:"viewers_list"`
		Payload      json.RawMessage        `json:"payload"`
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

	displayName := raw.DisplayName
	if displayName == "" {
		displayName = raw.UserName
	}

	roomType := raw.RoomType
	if roomType == "" && len(raw.Payload) > 0 {
		var payloadType struct {
			RoomType string `json:"room_type"`
		}
		if err := json.Unmarshal(raw.Payload, &payloadType); err == nil && payloadType.RoomType != "" {
			roomType = payloadType.RoomType
		}
	}

	metadata := raw.Metadata
	if metadata == nil && len(raw.Payload) > 0 {
		var payloadMeta struct {
			Metadata map[string]interface{} `json:"metadata"`
		}
		if err := json.Unmarshal(raw.Payload, &payloadMeta); err == nil && payloadMeta.Metadata != nil {
			metadata = payloadMeta.Metadata
		}
	}

	return &SignalingMessage{
		Action:       evt,
		Event:        evt,
		RoomID:       raw.RoomID,
		RoomName:     raw.RoomName,
		RoomType:     roomType,
		UserID:       raw.UserID,
		DisplayName:  displayName,
		AvatarURL:    raw.AvatarURL,
		Role:         raw.Role,
		Metadata:     metadata,
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
