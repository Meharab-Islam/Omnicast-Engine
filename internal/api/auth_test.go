package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

func TestAuthHandler_TokenGeneration(t *testing.T) {
	_ = os.Setenv("API_KEY", "test_key_123")
	_ = os.Setenv("API_SECRET", "test_secret_456")
	_ = os.Setenv("JWT_SECRET", "test_jwt_secret_789")

	handler := NewAuthHandler()

	app := fiber.New()
	app.Post("/api/auth/token", handler.HandleTokenGeneration)

	// 1. Test Valid Request
	validReq := TokenRequest{
		APIKey:    "test_key_123",
		APISecret: "test_secret_456",
		UserID:    "usr_alex",
		UserName:  "Alex Johnson",
		AvatarURL: "https://example.com/avatar.jpg",
	}
	body, _ := json.Marshal(validReq)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to execute request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200 OK, got %d", resp.StatusCode)
	}

	var resData TokenResponse
	_ = json.NewDecoder(resp.Body).Decode(&resData)
	if resData.Status != "success" || resData.Token == "" {
		t.Fatalf("Expected successful token response, got: %+v", resData)
	}

	// Validate the returned JWT token
	var claims UserClaims
	token, err := jwt.ParseWithClaims(resData.Token, &claims, func(tok *jwt.Token) (any, error) {
		return []byte("test_jwt_secret_789"), nil
	})
	if err != nil || !token.Valid {
		t.Fatalf("Generated token failed validation: %v", err)
	}
	if claims.UserID != "usr_alex" || claims.UserName != "Alex Johnson" || claims.AvatarURL != "https://example.com/avatar.jpg" {
		t.Fatalf("Claims mismatch: %+v", claims)
	}

	// Validate ICE Servers generated
	if len(resData.ICEServers) < 2 {
		t.Fatalf("Expected at least 2 ICE servers (STUN + TURN), got %d", len(resData.ICEServers))
	}
	hasTURN := false
	for _, s := range resData.ICEServers {
		if len(s.URLs) > 0 && s.Username != "" && s.Credential != "" {
			hasTURN = true
		}
	}
	if !hasTURN {
		t.Fatalf("Expected TURN server with credentials in ICE servers list: %+v", resData.ICEServers)
	}

	// 2. Test Invalid API Secret -> 401 Unauthorized
	invalidReq := TokenRequest{
		APIKey:    "test_key_123",
		APISecret: "wrong_secret",
		UserID:    "usr_alex",
	}
	body, _ = json.Marshal(invalidReq)
	req = httptest.NewRequest(http.MethodPost, "/api/auth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("Failed to execute invalid request: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected status 401 Unauthorized, got %d", resp.StatusCode)
	}

	// 3. Test Missing User ID -> 400 Bad Request
	missingUserReq := TokenRequest{
		APIKey:    "test_key_123",
		APISecret: "test_secret_456",
		UserID:    "",
	}
	body, _ = json.Marshal(missingUserReq)
	req = httptest.NewRequest(http.MethodPost, "/api/auth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("Failed to execute missing user request: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected status 400 Bad Request, got %d", resp.StatusCode)
	}
}

func TestAuthHandler_RequireAPIAuth(t *testing.T) {
	_ = os.Setenv("API_KEY", "test_key_123")
	_ = os.Setenv("API_SECRET", "test_secret_456")

	handler := NewAuthHandler()

	app := fiber.New()
	app.Get("/api/rooms", handler.RequireAPIAuth(), func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	// 1. Missing Headers -> 401 Unauthorized
	req := httptest.NewRequest(http.MethodGet, "/api/rooms", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed request: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected 401, got %d", resp.StatusCode)
	}

	// 2. Valid X-API-KEY and X-API-SECRET Headers -> 200 OK
	req = httptest.NewRequest(http.MethodGet, "/api/rooms", nil)
	req.Header.Set("X-API-KEY", "test_key_123")
	req.Header.Set("X-API-SECRET", "test_secret_456")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("Failed request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", resp.StatusCode)
	}

	// 3. Valid via Query Params -> 200 OK
	req = httptest.NewRequest(http.MethodGet, "/api/rooms?api_key=test_key_123&api_secret=test_secret_456", nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("Failed request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK via query params, got %d", resp.StatusCode)
	}
}
