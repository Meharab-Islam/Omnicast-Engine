package api

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// Default fallback secret keys if env is not provided
const (
	DefaultJWTSecret  = "live_media_server_jwt_secret_key_2026"
	DefaultTURNSecret = "my_super_secure_turn_secret_999"
)

// UserClaims defines the JWT payload structure for authenticated streaming
type UserClaims struct {
	UserID       string          `json:"user_id"`
	UserName     string          `json:"user_name,omitempty"`
	AvatarURL    string          `json:"avatar_url,omitempty"`
	Role         string          `json:"role,omitempty"`
	RoomID       string          `json:"room_id,omitempty"`
	CanPublish   *bool           `json:"can_publish,omitempty"`
	CanSubscribe *bool           `json:"can_subscribe,omitempty"`
	Permissions  map[string]bool `json:"permissions,omitempty"`
	jwt.RegisteredClaims
}

// ICEServerJSON represents a JSON-serializable ICE Server definition for SDK clients
type ICEServerJSON struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// TokenRequest represents the JSON body payload for /api/auth/token
type TokenRequest struct {
	APIKey       string `json:"api_key"`
	APISecret    string `json:"api_secret"`
	UserID       string `json:"user_id"`
	UserName     string `json:"user_name"`
	AvatarURL    string `json:"avatar_url"`
	Role         string `json:"role,omitempty"`
	RoomID       string `json:"room_id,omitempty"`
	CanPublish   *bool  `json:"can_publish,omitempty"`
	CanSubscribe *bool  `json:"can_subscribe,omitempty"`
}

// TokenResponse represents the JSON response returned by /api/auth/token
type TokenResponse struct {
	Status     string          `json:"status"`
	Token      string          `json:"token,omitempty"`
	UserID     string          `json:"user_id,omitempty"`
	UserName   string          `json:"user_name,omitempty"`
	AvatarURL  string          `json:"avatar_url,omitempty"`
	ExpiresIn  int64           `json:"expires_in,omitempty"`
	ICEServers []ICEServerJSON `json:"ice_servers,omitempty"`
	Error      string          `json:"error,omitempty"`
}

// AuthHandler handles authentication and token generation REST endpoints
type AuthHandler struct {
	apiKey     string
	apiSecret  string
	jwtSecret  string
	turnSecret string
	publicIP   string
}

// NewAuthHandler creates and initializes a new AuthHandler
func NewAuthHandler() *AuthHandler {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		apiKey = "dev_api_key_123"
	}
	apiSecret := os.Getenv("API_SECRET")
	if apiSecret == "" {
		apiSecret = "dev_api_secret_456"
	}
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = DefaultJWTSecret
	}
	turnSecret := os.Getenv("TURN_SECRET")
	if turnSecret == "" {
		turnSecret = DefaultTURNSecret
	}
	publicIP := os.Getenv("PUBLIC_IP")
	if publicIP == "" {
		publicIP = "192.168.0.116"
	}

	return &AuthHandler{
		apiKey:     apiKey,
		apiSecret:  apiSecret,
		jwtSecret:  jwtSecret,
		turnSecret: turnSecret,
		publicIP:   publicIP,
	}
}

// GenerateUserToken generates a signed JWT token containing user profile details
func GenerateUserToken(userID, userName, avatarURL, secret string, duration time.Duration) (string, error) {
	return GenerateTokenWithPermissions(userID, userName, avatarURL, "user", "", nil, nil, secret, duration)
}

// GenerateTokenWithPermissions generates a signed JWT token containing custom permissions (can_publish, can_subscribe)
func GenerateTokenWithPermissions(userID, userName, avatarURL, role, roomID string, canPublish, canSubscribe *bool, secret string, duration time.Duration) (string, error) {
	if secret == "" {
		secret = os.Getenv("JWT_SECRET")
		if secret == "" {
			secret = DefaultJWTSecret
		}
	}
	if role == "" {
		role = "user"
	}

	claims := UserClaims{
		UserID:       userID,
		UserName:     userName,
		AvatarURL:    avatarURL,
		Role:         role,
		RoomID:       roomID,
		CanPublish:   canPublish,
		CanSubscribe: canSubscribe,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "omnicast",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// GenerateTURNCredentials generates standard time-limited HMAC-SHA1 TURN credentials (TURN REST API)
func (h *AuthHandler) GenerateTURNCredentials(userID string, duration time.Duration) (string, string) {
	if userID == "" {
		userID = "anonymous"
	}
	expiry := time.Now().Add(duration).Unix()
	username := fmt.Sprintf("%d:%s", expiry, userID)

	mac := hmac.New(sha1.New, []byte(h.turnSecret))
	mac.Write([]byte(username))
	password := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return username, password
}

// GenerateICEServers constructs STUN & dynamic TURN REST API ice_servers list
func (h *AuthHandler) GenerateICEServers(userID string) []ICEServerJSON {
	username, password := h.GenerateTURNCredentials(userID, 24*time.Hour)

	return []ICEServerJSON{
		{
			URLs: []string{
				"stun:stun.l.google.com:19302",
				fmt.Sprintf("stun:%s:3478", h.publicIP),
			},
		},
		{
			URLs: []string{
				fmt.Sprintf("turn:%s:3478?transport=udp", h.publicIP),
				fmt.Sprintf("turn:%s:3478?transport=tcp", h.publicIP),
			},
			Username:   username,
			Credential: password,
		},
	}
}

// HandleTokenGeneration processes POST /api/auth/token requests
func (h *AuthHandler) HandleTokenGeneration(c *fiber.Ctx) error {
	var req TokenRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(TokenResponse{
			Status: "error",
			Error:  "Invalid JSON request body",
		})
	}

	// Validate API Key and API Secret
	if req.APIKey == "" || req.APISecret == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(TokenResponse{
			Status: "error",
			Error:  "Unauthorized: api_key and api_secret are required",
		})
	}

	if req.APIKey != h.apiKey || req.APISecret != h.apiSecret {
		return c.Status(fiber.StatusUnauthorized).JSON(TokenResponse{
			Status: "error",
			Error:  "Unauthorized: invalid api_key or api_secret",
		})
	}

	// Validate User ID
	if req.UserID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(TokenResponse{
			Status: "error",
			Error:  "user_id is required",
		})
	}

	// Generate 24-hour JWT token with granular permissions
	token, err := GenerateTokenWithPermissions(req.UserID, req.UserName, req.AvatarURL, req.Role, req.RoomID, req.CanPublish, req.CanSubscribe, h.jwtSecret, 24*time.Hour)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(TokenResponse{
			Status: "error",
			Error:  "Failed to generate JWT: " + err.Error(),
		})
	}

	// Generate dynamic time-limited TURN REST credentials for NAT traversal
	iceServers := h.GenerateICEServers(req.UserID)

	return c.Status(fiber.StatusOK).JSON(TokenResponse{
		Status:     "success",
		Token:      token,
		UserID:     req.UserID,
		UserName:   req.UserName,
		AvatarURL:  req.AvatarURL,
		ExpiresIn:  86400,
		ICEServers: iceServers,
	})
}
