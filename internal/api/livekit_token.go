package api

import (
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/livekit/protocol/auth"
)

// LiveKitTokenRequest represents the request body for generating a LiveKit AccessToken
type LiveKitTokenRequest struct {
	APIKey       string `json:"api_key"`
	APISecret    string `json:"api_secret"`
	RoomID       string `json:"room_id"`
	UserID       string `json:"user_id"`
	UserName     string `json:"user_name"`
	AvatarURL    string `json:"avatar_url"`
	Role         string `json:"role"` // "host", "cohost", "viewer"
	CanPublish   *bool  `json:"can_publish,omitempty"`
	CanSubscribe *bool  `json:"can_subscribe,omitempty"`
}

// LiveKitTokenResponse represents the JSON response with LiveKit AccessToken and Server URL
type LiveKitTokenResponse struct {
	Status     string `json:"status"`
	Success    bool   `json:"success"`
	Token      string `json:"token,omitempty"`
	ServerURL  string `json:"server_url,omitempty"`
	LiveKitURL string `json:"livekit_url,omitempty"`
	RoomID     string `json:"room_id,omitempty"`
	UserID     string `json:"user_id,omitempty"`
	Role       string `json:"role,omitempty"`
	ExpiresIn  int64  `json:"expires_in,omitempty"`
	Error      string `json:"error,omitempty"`
}

// GenerateLiveKitToken creates a signed LiveKit AccessToken with appropriate VideoGrant permissions
func GenerateLiveKitToken(apiKey, apiSecret, roomID, userID, userName, role string, canPublish, canSubscribe *bool, duration time.Duration) (string, error) {
	if apiKey == "" {
		apiKey = os.Getenv("LIVEKIT_API_KEY")
		if apiKey == "" {
			apiKey = os.Getenv("API_KEY")
			if apiKey == "" {
				apiKey = "devkey"
			}
		}
	}
	if apiSecret == "" {
		apiSecret = os.Getenv("LIVEKIT_API_SECRET")
		if apiSecret == "" {
			apiSecret = os.Getenv("API_SECRET")
			if apiSecret == "" {
				apiSecret = "secret"
			}
		}
	}
	if duration <= 0 {
		duration = 24 * time.Hour
	}
	if userID == "" {
		userID = "anonymous"
	}
	if userName == "" {
		userName = userID
	}

	at := auth.NewAccessToken(apiKey, apiSecret)
	grant := &auth.VideoGrant{
		Room:     roomID,
		RoomJoin: true,
	}

	// 1. Host Permissions: RoomJoin: true, CanPublish: true, CanSubscribe: true, RoomAdmin: true
	if role == "host" || role == "broadcaster" || role == "publisher" || role == "admin" {
		grant.SetCanPublish(true)
		grant.SetCanSubscribe(true)
		grant.SetCanPublishData(true)
		grant.RoomAdmin = true
	} else if role == "cohost" || role == "co_host" {
		// Co-host Permissions: RoomJoin: true, CanPublish: true, CanSubscribe: true
		grant.SetCanPublish(true)
		grant.SetCanSubscribe(true)
		grant.SetCanPublishData(true)
	} else {
		// 2. Viewer Permissions: RoomJoin: true, CanPublish: false, CanSubscribe: true
		grant.SetCanPublish(false)
		grant.SetCanSubscribe(true)
		grant.SetCanPublishData(true) // Required for chat messages and reactions
	}

	// Explicit permission overrides if passed in request
	if canPublish != nil {
		grant.SetCanPublish(*canPublish)
	}
	if canSubscribe != nil {
		grant.SetCanSubscribe(*canSubscribe)
	}

	at.AddGrant(grant).
		SetIdentity(userID).
		SetName(userName).
		SetValidFor(duration)

	return at.ToJWT()
}

// HandleLiveKitToken processes POST /api/livekit/token requests
func (h *AuthHandler) HandleLiveKitToken(c *fiber.Ctx) error {
	var req LiveKitTokenRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(LiveKitTokenResponse{
			Status:  "error",
			Success: false,
			Error:   "Invalid JSON request body: " + err.Error(),
		})
	}

	// Validate API Key and Secret
	if req.APIKey != "" || req.APISecret != "" {
		if req.APIKey != h.apiKey || req.APISecret != h.apiSecret {
			return c.Status(fiber.StatusUnauthorized).JSON(LiveKitTokenResponse{
				Status:  "error",
				Success: false,
				Error:   "Unauthorized: invalid api_key or api_secret",
			})
		}
	} else if !h.ValidateAPIKeySecret(c) {
		// Check headers if not in body
		return c.Status(fiber.StatusUnauthorized).JSON(LiveKitTokenResponse{
			Status:  "error",
			Success: false,
			Error:   "Unauthorized: valid X-API-KEY and X-API-SECRET required",
		})
	}

	if req.RoomID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(LiveKitTokenResponse{
			Status:  "error",
			Success: false,
			Error:   "room_id is required",
		})
	}
	if req.UserID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(LiveKitTokenResponse{
			Status:  "error",
			Success: false,
			Error:   "user_id is required",
		})
	}

	role := req.Role
	if role == "" {
		role = "viewer"
	}

	// LiveKit API Key and Secret
	livekitKey := os.Getenv("LIVEKIT_API_KEY")
	if livekitKey == "" {
		livekitKey = h.apiKey
	}
	livekitSecret := os.Getenv("LIVEKIT_API_SECRET")
	if livekitSecret == "" {
		livekitSecret = h.apiSecret
	}

	token, err := GenerateLiveKitToken(livekitKey, livekitSecret, req.RoomID, req.UserID, req.UserName, role, req.CanPublish, req.CanSubscribe, 24*time.Hour)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(LiveKitTokenResponse{
			Status:  "error",
			Success: false,
			Error:   "Failed to generate LiveKit AccessToken: " + err.Error(),
		})
	}

	livekitURL := os.Getenv("LIVEKIT_URL")
	if livekitURL == "" {
		livekitURL = os.Getenv("LIVEKIT_SERVER_URL")
		if livekitURL == "" {
			livekitURL = "ws://" + h.publicIP + ":7880"
		}
	}

	return c.Status(fiber.StatusOK).JSON(LiveKitTokenResponse{
		Status:     "success",
		Success:    true,
		Token:      token,
		ServerURL:  livekitURL,
		LiveKitURL: livekitURL,
		RoomID:     req.RoomID,
		UserID:     req.UserID,
		Role:       role,
		ExpiresIn:  86400,
	})
}
