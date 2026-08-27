package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"live-media-server/internal/api"
	"live-media-server/internal/broker"
	"live-media-server/internal/signaling"
	"live-media-server/internal/webrtc"
)

func main() {
	// Initialize Pion WebRTC API with VP8, H264, Opus codecs and interceptors
	webrtcAPI, err := webrtc.InitWebRTC()
	if err != nil {
		log.Fatalf("Failed to initialize WebRTC API: %v\n", err)
	}

	// Initialize Webhook Dispatcher
	webhookDispatcher := api.NewWebhookDispatcher()
	webhookDispatcher.Start()

	// Initialize RoomManager, PKManager, and Signaling Hub
	roomManager := signaling.NewRoomManager()
	roomManager.SetWebhookDispatcher(webhookDispatcher)

	// Initialize Redis Pub/Sub Broker if REDIS_ADDR is configured
	redisAddr := getEnv("REDIS_ADDR", "")
	redisPass := getEnv("REDIS_PASSWORD", "")
	if redisAddr != "" {
		redisBroker, err := broker.NewRedisBroker(redisAddr, redisPass, 0)
		if err != nil {
			log.Printf("[Redis Warning] Failed to connect to Redis at %s: %v. Running in standalone in-memory mode.\n", redisAddr, err)
		} else {
			roomManager.SetBroker(redisBroker)
			log.Printf("[Redis] Distributed Pub/Sub broker activated on %s.\n", redisAddr)
		}
	} else {
		log.Println("[Redis] REDIS_ADDR not configured. Running in standalone in-memory mode.")
	}

	// Read server role and identification environment variables
	port := getEnv("PORT", "8080")
	publicIP := getEnv("PUBLIC_IP", "192.168.0.116")
	serverRole := getEnv("SERVER_ROLE", "origin")
	serverID := getEnv("SERVER_ID", "server-node-1")
	serverPublicAddr := getEnv("SERVER_PUBLIC_ADDR", fmt.Sprintf("ws://%s:%s/ws", publicIP, port))

	roomManager.SetServerConfig(serverRole, serverPublicAddr)
	log.Printf("[Server Init] Role: %s | ID: %s | Public Address: %s\n", serverRole, serverID, serverPublicAddr)

	// Initialize CascadeManager for SFU Cascading (Inter-Server WebRTC)
	cascadeManager := webrtc.NewCascadeManager(webrtcAPI, roomManager.GetBroker(), serverID)
	roomManager.SetCascadeManager(cascadeManager)

	pkManager := signaling.NewPKManager(roomManager)
	roomManager.SetPKManager(pkManager)

	hub := signaling.NewHub(roomManager)
	go hub.Run()

	app := fiber.New(fiber.Config{
		AppName: "Live Media Server v1.0",
	})

	// CORS Middleware to allow Flutter, React, and other frontends to communicate
	app.Use(cors.New(cors.Config{
		AllowOrigins:     "*",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, Sec-WebSocket-Protocol",
		AllowMethods:     "GET, POST, HEAD, PUT, DELETE, PATCH, OPTIONS",
		AllowCredentials: false,
	}))

	// Read environment variables for STUN/TURN configuration
	turnURL := getEnv("TURN_URL", "turn:127.0.0.1:3478")
	turnUsername := getEnv("TURN_USERNAME", "myuser")
	turnPassword := getEnv("TURN_PASSWORD", "mypassword")

	// Serve static web client from ./public folder
	app.Static("/", "./public")

	// GET /api/ice-servers - Returns STUN and TURN server configuration for WebRTC clients
	app.Get("/api/ice-servers", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"iceServers": []fiber.Map{
				{
					"urls": "stun:stun.l.google.com:19302",
				},
				{
					"urls":       turnURL,
					"username":   turnUsername,
					"credential": turnPassword,
				},
			},
		})
	})

	// Health Check Endpoint
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status":       "ok",
			"message":      "Live Media Server is running",
			"active_rooms": roomManager.ActiveRoomsCount(),
			"webhook": fiber.Map{
				"configured": webhookDispatcher.WebhookURL != "",
			},
		})
	})

	// Initialize REST Auth Handler for API Key / Secret validation and JWT generation
	authHandler := api.NewAuthHandler()
	app.Post("/api/auth/token", authHandler.HandleTokenGeneration)

	// GET /auth/demo-token - Generates a signed JWT token for test clients
	app.Get("/auth/demo-token", func(c *fiber.Ctx) error {
		userID := c.Query("user_id", "user-"+time.Now().Format("150405"))
		role := c.Query("role", "viewer")
		roomID := c.Query("room_id", "room-101")

		token, err := signaling.GenerateToken(userID, role, roomID, "", 24*time.Hour)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"error":   err.Error(),
			})
		}

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"success": true,
			"token":   token,
			"user_id": userID,
			"role":    role,
			"room_id": roomID,
		})
	})

	// GET /rooms - Return list of all currently active rooms with room_id, room_name, host_id, and viewer_count
	app.Get("/rooms", func(c *fiber.Ctx) error {
		rooms := roomManager.GetAllRooms()
		return c.Status(fiber.StatusOK).JSON(rooms)
	})

	// GET /api/admin/rooms - Secured Admin Endpoint to fetch server-wide stats, active room details, user counts, and session uptimes
	app.Get("/api/admin/rooms", func(c *fiber.Ctx) error {
		apiKey := c.Get("X-API-Key")
		if apiKey == "" {
			apiKey = c.Query("api_key")
		}
		expectedKey := os.Getenv("API_KEY")
		if expectedKey == "" {
			expectedKey = "dev_api_key_123"
		}

		if apiKey != expectedKey {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"status": "error",
				"error":  "Unauthorized: valid X-API-Key header or api_key query param required",
			})
		}

		rooms := roomManager.GetAllRoomsSummary()
		type AdminRoomStat struct {
			RoomID        string `json:"room_id"`
			RoomName      string `json:"room_name"`
			HostID        string `json:"host_id"`
			TotalViewers  int    `json:"total_viewers"`
			HostScore     int    `json:"host_score"`
			CreatedAt     string `json:"created_at"`
			UptimeSeconds int64  `json:"uptime_seconds"`
		}

		var stats []AdminRoomStat
		now := time.Now().UTC()
		for _, r := range rooms {
			var uptime int64
			if parsedTime, err := time.Parse(time.RFC3339, r.CreatedAt); err == nil {
				uptime = int64(now.Sub(parsedTime).Seconds())
			}
			stats = append(stats, AdminRoomStat{
				RoomID:        r.RoomID,
				RoomName:      r.RoomName,
				HostID:        r.HostID,
				TotalViewers:  r.ViewersCount,
				HostScore:     r.HostScore,
				CreatedAt:     r.CreatedAt,
				UptimeSeconds: uptime,
			})
		}

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status":             "success",
			"total_active_rooms": len(stats),
			"timestamp":          now.Unix(),
			"rooms":              stats,
		})
	})

	// GET /room/:id - Return details for a specific active room
	app.Get("/room/:id", func(c *fiber.Ctx) error {
		roomID := c.Params("id")
		if roomID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"success": false,
				"error":   "room id is required",
			})
		}

		summary, exists := roomManager.GetRoomSummary(roomID)
		if !exists || summary == nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"success": false,
				"error":   "room not found",
			})
		}

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"success": true,
			"room":    summary,
		})
	})

	// Server Secret for Server-to-Server Cascading Auth Bypass
	serverSecret := getEnv("SERVER_SECRET", "super_secret_cascade_key_123")

	// WebSocket Upgrade and JWT Authentication Check Middleware
	app.Use("/ws", func(c *fiber.Ctx) error {
		if !websocket.IsWebSocketUpgrade(c) {
			return fiber.ErrUpgradeRequired
		}

		// 1. Check for Server Secret bypass (Edge Server -> Origin Server cascading)
		providedSecret := c.Query("server_secret")
		if providedSecret == "" {
			providedSecret = c.Query("secret")
		}
		if providedSecret == "" {
			providedSecret = c.Get("X-Server-Secret")
		}

		if serverSecret != "" && providedSecret == serverSecret {
			edgeServerClaims := &signaling.UserClaims{
				UserID: "edge-" + c.Query("server_id", "cascade-node"),
				Role:   "edge_server",
			}
			c.Locals("allowed", true)
			c.Locals("user_claims", edgeServerClaims)
			log.Printf("[Auth Bypass] Authenticated Inter-Server Cascading Connection: UserID=%s, Role=%s\n",
				edgeServerClaims.UserID, edgeServerClaims.Role)
			return c.Next()
		}

		// 2. Extract token from query parameter "?token=..." or Authorization header
		tokenStr := c.Query("token")
		if tokenStr == "" {
			authHeader := c.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
			} else {
				tokenStr = authHeader
			}
		}

		// Fallback for WebSocket subprotocol token
		if tokenStr == "" {
			tokenStr = c.Get("Sec-WebSocket-Protocol")
		}

		// Validate JWT token using JWT_SECRET
		claims, err := signaling.ValidateToken(tokenStr, "")
		if err != nil {
			log.Printf("WebSocket connection rejected: 401 Unauthorized (%v)\n", err)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"error":   "Unauthorized: " + err.Error(),
			})
		}

		c.Locals("allowed", true)
		c.Locals("user_claims", claims)
		return c.Next()
	})

	// Fiber Native WebSocket Route with authenticated claims
	app.Get("/ws", websocket.New(func(c *websocket.Conn) {
		claims, _ := c.Locals("user_claims").(*signaling.UserClaims)
		client := signaling.NewClientWithClaims(hub, roomManager, webrtcAPI, c, claims)
		hub.Register() <- client

		go client.WritePump()
		client.ReadPump() // blocks and manages lifecycle until disconnection
	}))

	// Channel to capture OS signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in a separate goroutine
	go func() {
		log.Printf("Live Media Server starting on port :%s...\n", port)
		if err := app.Listen(":" + port); err != nil {
			log.Printf("Server stopped listening: %v\n", err)
		}
	}()

	// Block until an interrupt or termination signal is received
	sig := <-sigChan
	log.Printf("Received signal: %v. Initiating graceful shutdown...\n", sig)

	// Gracefully stop Webhook Dispatcher
	webhookDispatcher.Stop()

	// Gracefully close SFU Cascade sessions
	if cm := roomManager.GetCascadeManager(); cm != nil {
		cm.Close()
	}

	// Gracefully close Redis Broker connection
	if b := roomManager.GetBroker(); b != nil {
		_ = b.Close()
	}

	// Gracefully shutdown the Fiber app with a 5-second timeout
	if err := app.ShutdownWithTimeout(5 * time.Second); err != nil {
		log.Fatalf("Server forced to shutdown: %v\n", err)
	}

	log.Println("Server exited gracefully.")
}

// getEnv retrieves the value of the environment variable named by key, or fallback default if unset or empty
func getEnv(key, defaultVal string) string {
	if val, exists := os.LookupEnv(key); exists && val != "" {
		return val
	}
	return defaultVal
}
