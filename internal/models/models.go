package models

import "time"

// Participant represents an active member (Host, Co-Host, or Viewer) with rich dynamic profile metadata
type Participant struct {
	UserID      string                 `json:"user_id"`
	ID          string                 `json:"id,omitempty"`
	DisplayName string                 `json:"display_name"`
	Name        string                 `json:"name,omitempty"`
	AvatarURL   string                 `json:"avatar_url,omitempty"`
	Role        string                 `json:"role,omitempty"` // "host", "cohost", "viewer"
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	JoinedAt    time.Time              `json:"joined_at,omitempty"`
}
