package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/pion/turn/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"omnicast/internal/api"
	"omnicast/internal/broker"
	"omnicast/internal/config"
	"omnicast/internal/metrics"
	"omnicast/internal/models"
	"omnicast/internal/signaling"
	"omnicast/internal/webrtc"
)

// isServerDraining is a globally accessible atomic boolean initialized to false,
// indicating whether the media server is currently draining connections during graceful shutdown.
var isServerDraining atomic.Bool

func main() {
	// Zero-Config Boot: Auto-generate secure credentials, resolve public IP, and persist to .env
	cfg := config.LoadOrGenerateConfig()

	// To handle strict firewalls that block UDP entirely, create a TCP listener on port 443 using net.Listen("tcp", ":443")
	turnTCPListener, turnTCPErr := net.Listen("tcp", ":443")
	if turnTCPErr != nil {
		log.Printf("[TURN Info] TCP port :443 status: %v (requires elevated permissions or reverse proxy for strict firewall bypass)\n", turnTCPErr)
	} else {
		defer turnTCPListener.Close()
		log.Println("[TURN] TCP listener active on port 443 (:443) for strict firewall bypass")
	}

	// Initialize Pion WebRTC API with VP8, H264, VP9, Opus codecs and apply SetICETCPMux()
	webrtcAPI, err := webrtc.InitWebRTCWithTCPListener(turnTCPListener)
	if err != nil {
		log.Fatalf("Failed to initialize WebRTC API: %v\n", err)
	}

	// Initialize Webhook Dispatcher
	webhookDispatcher := api.NewWebhookDispatcher()
	webhookDispatcher.Start()

	// Initialize RoomManager, PKManager, and Signaling Hub
	roomManager := signaling.NewRoomManager()
	roomManager.SetWebRTCAPI(webrtcAPI)
	roomManager.SetWebhookDispatcher(webhookDispatcher)

	// Initialize Redis Client connection (github.com/redis/go-redis/v9)
	var redisClient *redis.Client
	if cfg.RedisAddr != "" {
		redisClient = redis.NewClient(&redis.Options{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPass,
			DB:       0,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if pingErr := redisClient.Ping(ctx).Err(); pingErr != nil {
			log.Printf("[go-redis] Ping failed on %s: %v (running in standalone mode)\n", cfg.RedisAddr, pingErr)
		} else {
			log.Printf("[go-redis] Connected to Redis instance at %s\n", cfg.RedisAddr)
		}
		cancel()

		// Initialize Redis Pub/Sub Broker
		redisBroker, err := broker.NewRedisBroker(cfg.RedisAddr, cfg.RedisPass, 0)
		if err != nil {
			log.Printf("[Redis Warning] Failed to initialize broker at %s: %v. Running in standalone in-memory mode.\n", cfg.RedisAddr, err)
		} else {
			roomManager.SetBroker(redisBroker)
			log.Printf("[Redis] Distributed Pub/Sub broker activated on %s.\n", cfg.RedisAddr)
		}
	} else {
		log.Println("[Redis] REDIS_ADDR not configured. Running in standalone in-memory mode.")
	}

	serverPublicAddr := getEnv("SERVER_PUBLIC_ADDR", fmt.Sprintf("ws://%s:%s/ws", cfg.PublicIP, cfg.Port))
	roomManager.SetServerConfig(cfg.ServerRole, serverPublicAddr)
	roomManager.SetNodeID(cfg.ServerID)
	log.Printf("[Server Init] Role: %s | ID: %s | Public Address: %s\n", cfg.ServerRole, cfg.ServerID, serverPublicAddr)

	// Initialize CascadeManager for SFU Cascading (Inter-Server WebRTC)
	cascadeManager := webrtc.NewCascadeManager(webrtcAPI, roomManager.GetBroker(), cfg.ServerID)
	roomManager.SetCascadeManager(cascadeManager)

	pkManager := signaling.NewPKManager(roomManager)
	roomManager.SetPKManager(pkManager)

	hub := signaling.NewHub(roomManager)
	hub.StartZombiePeerGC(10*time.Second, 15*time.Second)
	go hub.Run()

	app := fiber.New(fiber.Config{
		AppName: "OmniCast Engine v1.0",
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

	// Create a standard UDP listener on port 3478 using net.ListenPacket for TURN/STUN NAT traversal
	var embeddedTURNServer *turn.Server
	turnUDPListener, turnErr := net.ListenPacket("udp4", "0.0.0.0:3478")

	if turnErr != nil {
		log.Printf("[TURN Info] UDP port 3478 status: %v (running with external TURN or port in use)\n", turnErr)
	} else {
		publicIP := getEnv("PUBLIC_IP", "127.0.0.1")
		realm := getEnv("TURN_REALM", "omnicast.live")
		authSecret := getEnv("TURN_SECRET", "omnicast_secret_turn_key")

		relayGen := &turn.RelayAddressGeneratorPortRange{
			RelayAddress: net.ParseIP(publicIP),
			Address:      "0.0.0.0",
			MinPort:      50000,
			MaxPort:      50200,
		}

		listenerConfigs := []turn.ListenerConfig{}
		if turnTCPListener != nil {
			listenerConfigs = append(listenerConfigs, turn.ListenerConfig{
				Listener:              turnTCPListener,
				RelayAddressGenerator: relayGen,
			})
		}

		server, err := turn.NewServer(turn.ServerConfig{
			Realm: realm,
			// Dynamic AuthHandler: Validate temporary, time-limited credentials using shared secret
			AuthHandler: func(username string, realm string, srcAddr net.Addr) ([]byte, bool) {
				return webrtc.ValidateAndGenerateAuthKey(username, realm, authSecret)
			},
			PacketConnConfigs: []turn.PacketConnConfig{
				{
					PacketConn:            turnUDPListener,
					RelayAddressGenerator: relayGen,
				},
			},
			ListenerConfigs: listenerConfigs,
		})
		if err != nil {
			log.Printf("[TURN Error] Failed to initialize embedded TURN server: %v\n", err)
			_ = turnUDPListener.Close()
		} else {
			embeddedTURNServer = server
			log.Printf("[TURN] Embedded STUN/TURN server initialized on UDP 0.0.0.0:3478 (Realm: %s, Public IP: %s)\n", realm, publicIP)
		}
	}

	// Serve static web client from ./public folder
	app.Static("/", "./public")

	// GET /api/ice-servers - Returns STUN and TURN server configuration with temporary time-limited credentials
	app.Get("/api/ice-servers", func(c *fiber.Ctx) error {
		userID := c.Query("user_id", "user-"+time.Now().Format("150405"))
		authSecret := getEnv("TURN_SECRET", "omnicast_secret_turn_key")

		// Generate temporary time-limited credentials (valid for 24 hours) using HMAC-SHA1 shared secret
		expiry := time.Now().Add(24 * time.Hour).Unix()
		ephemeralUsername := fmt.Sprintf("%d:%s", expiry, userID)

		mac := hmac.New(sha1.New, []byte(authSecret))
		mac.Write([]byte(ephemeralUsername))
		ephemeralPassword := base64.StdEncoding.EncodeToString(mac.Sum(nil))

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"iceServers": []fiber.Map{
				{
					"urls": "stun:stun.l.google.com:19302",
				},
				{
					"urls":       turnURL,
					"username":   ephemeralUsername,
					"credential": ephemeralPassword,
				},
			},
		})
	})

	// GET & POST /turn_credentials - Returns JSON object containing TURN server URIs, username, and password
	handleTURNCredentials := func(c *fiber.Ctx) error {
		userID := c.Query("user_id")
		if userID == "" {
			var body struct {
				UserID string `json:"user_id"`
			}
			_ = c.BodyParser(&body)
			userID = body.UserID
		}
		if userID == "" {
			userID = "user-" + time.Now().Format("150405")
		}

		authSecret := getEnv("TURN_SECRET", "omnicast_secret_turn_key")
		publicDomain := getEnv("TURN_DOMAIN", getEnv("PUBLIC_IP", "127.0.0.1"))
		turnPort := getEnv("TURN_PORT", "3478")

		// Generate temporary time-limited credentials (valid for 24 hours) using HMAC-SHA1
		ttlSec := int64(86400) // 24 hours
		expiry := time.Now().Unix() + ttlSec
		ephemeralUsername := fmt.Sprintf("%d:%s", expiry, userID)

		mac := hmac.New(sha1.New, []byte(authSecret))
		mac.Write([]byte(ephemeralUsername))
		ephemeralPassword := base64.StdEncoding.EncodeToString(mac.Sum(nil))

		turnURIs := []string{
			fmt.Sprintf("turn:%s:%s", publicDomain, turnPort),
			fmt.Sprintf("turn:%s:443?transport=tcp", publicDomain),
		}

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"uris":     turnURIs,
			"username": ephemeralUsername,
			"password": ephemeralPassword,
			"ttl":      ttlSec,
		})
	}

	app.Get("/turn_credentials", handleTURNCredentials)
	app.Post("/turn_credentials", handleTURNCredentials)
	app.Get("/api/turn_credentials", handleTURNCredentials)
	app.Post("/api/turn_credentials", handleTURNCredentials)

	// Health Check Endpoint (returns 503 Service Unavailable when draining for Load Balancer failover)
	// Returns HTTP 200 OK along with quick JSON summary of CPU/Memory usage and active rooms for load balancers
	app.Get("/health", func(c *fiber.Ctx) error {
		sysSummary := metrics.GetSystemSummary()
		if isServerDraining.Load() {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"status":       "draining",
				"message":      "Server is draining connections for shutdown/maintenance",
				"active_rooms": roomManager.ActiveRoomsCount(),
				"draining":     true,
				"system":       sysSummary,
			})
		}

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status":       "ok",
			"message":      "Live Media Server is running",
			"active_rooms": roomManager.ActiveRoomsCount(),
			"draining":     false,
			"system":       sysSummary,
			"webhook": fiber.Map{
				"configured": webhookDispatcher.WebhookURL != "",
			},
		})
	})

	// Prometheus Metrics Endpoint for monitoring and scraping
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

	// Initialize REST Auth Handler for API Key / Secret validation and JWT generation
	authHandler := api.NewAuthHandler()
	app.Post("/api/auth/token", authHandler.HandleTokenGeneration)

	// POST /api/gift - Targeted Gifting REST API
	app.Post("/api/gift", func(c *fiber.Ctx) error {
		var req struct {
			RoomID       string `json:"room_id"`
			SenderID     string `json:"sender_id"`
			SenderName   string `json:"sender_name"`
			GiftID       string `json:"gift_id"`
			Gift         string `json:"gift"`
			TargetHostID string `json:"target_host_id"`
			ReceiverID   string `json:"receiver_id"`
			Coins        int    `json:"coins"`
			Points       int    `json:"points"`
			Amount       int    `json:"amount"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"success": false,
				"error":   "Invalid request body",
			})
		}
		if req.RoomID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"success": false,
				"error":   "room_id is required",
			})
		}

		payloadBytes, _ := json.Marshal(req)
		msg := &models.SignalingMessage{
			Action:  "gift",
			Event:   "gift",
			RoomID:  req.RoomID,
			UserID:  req.SenderID,
			Payload: payloadBytes,
		}

		dummyClient := &signaling.Client{
			ID:          req.SenderID,
			UserName:    req.SenderName,
			RoomID:      req.RoomID,
			RoomManager: roomManager,
		}
		dummyClient.HandleGiftMessageDirect(msg)

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"success": true,
			"message": "Gift processed successfully",
		})
	})

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

	// POST /api/admin/rooms/:id/end - Admin Kill Switch to forcefully terminate and destroy a room
	adminEndHandler := func(c *fiber.Ctx) error {
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

		roomID := c.Params("id")
		if roomID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"status": "error",
				"error":  "Room ID parameter is required",
			})
		}

		if _, exists := roomManager.GetRoom(roomID); !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"status": "error",
				"error":  "Room not found or already ended",
			})
		}

		roomManager.ForceEndRoom(roomID, "admin", "closed_by_admin")

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status":    "success",
			"message":   "Room forcefully ended and destroyed successfully",
			"room_id":   roomID,
			"timestamp": time.Now().UTC().Unix(),
		})
	}
	app.Post("/api/admin/rooms/:id/end", adminEndHandler)
	app.Post("/api/admin/rooms/:id/close", adminEndHandler)

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

		// If server is in draining mode and a new host connects to create a room, return HTTP 503 Service Unavailable
		if isServerDraining.Load() && (claims.Role == "host" || claims.Role == "publisher" || c.Query("role") == "host") {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"success": false,
				"status":  503,
				"error":   "Service Unavailable: Server is draining",
			})
		}

		c.Locals("allowed", true)
		c.Locals("user_claims", claims)
		return c.Next()
	})

	// POST /api/rooms - HTTP Endpoint to create a new room (returns 503 if server is draining)
	app.Post("/api/rooms", func(c *fiber.Ctx) error {
		if isServerDraining.Load() {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"success": false,
				"status":  503,
				"error":   "Service Unavailable: Server is currently draining",
			})
		}

		var req struct {
			RoomID   string `json:"room_id"`
			HostID   string `json:"host_id"`
			RoomName string `json:"room_name"`
			RoomType string `json:"room_type"`
		}
		if err := c.BodyParser(&req); err != nil || req.RoomID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"success": false,
				"error":   "invalid request body or missing room_id",
			})
		}
		if req.HostID == "" {
			req.HostID = "host-" + req.RoomID
		}

		room, err := roomManager.CreateRoom(req.RoomID, req.HostID)
		if err != nil {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"success": false,
				"error":   err.Error(),
			})
		}
		if req.RoomName != "" {
			room.SetRoomName(req.RoomName)
		}
		if req.RoomType != "" {
			room.SetRoomType(req.RoomType)
		}

		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"success": true,
			"room": fiber.Map{
				"room_id":   room.RoomID,
				"host_id":   room.HostID,
				"room_name": room.GetRoomName(),
				"room_type": room.GetRoomType(),
			},
		})
	})
	app.Post("/api/room", func(c *fiber.Ctx) error {
		if isServerDraining.Load() {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"success": false,
				"status":  503,
				"error":   "Service Unavailable: Server is currently draining",
			})
		}
		return c.Redirect("/api/rooms", fiber.StatusTemporaryRedirect)
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
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a separate goroutine
	go func() {
		if err := app.Listen(":" + cfg.Port); err != nil {
			log.Printf("Server stopped listening: %v\n", err)
		}
	}()

	// Small pause to let listener start, then print styled zero-config banner
	time.Sleep(100 * time.Millisecond)
	config.PrintStartupBanner(cfg)

	// Block until an interrupt or termination signal is received
	sig := <-sigCh
	log.Printf("Received signal: %v. Initiating graceful shutdown...\n", sig)
	log.Println("Server entering drain mode")
	isServerDraining.Store(true)
	signaling.SetServerDraining(true)

	// Hard-timeout fallback: Forcefully call os.Exit(0) after 2 hours if rooms do not naturally empty out
	drainTimeout := 2 * time.Hour
	fallbackTimer := time.AfterFunc(drainTimeout, func() {
		log.Printf("[Drain Hard-Timeout] 2-hour drain window expired with %d active room(s). Forcefully terminating server process...\n",
			roomManager.ActiveRoomsCount())
		os.Exit(0)
	})
	defer fallbackTimer.Stop()

	// Block the main thread using wg.Wait() so the process stays alive for existing streams
	activeCount := roomManager.ActiveRoomsCount()
	if activeCount > 0 {
		log.Printf("Waiting for %d active room(s) to finish streaming naturally (up to 2h)...\n", activeCount)
		roomManager.GetActiveRoomsWG().Wait()
		log.Println("All active rooms have finished naturally. Proceeding with shutdown...")
	}
	fallbackTimer.Stop()

	// Gracefully stop Webhook Dispatcher
	webhookDispatcher.Stop()

	// Gracefully close embedded TURN server
	if embeddedTURNServer != nil {
		_ = embeddedTURNServer.Close()
	}

	// Gracefully close SFU Cascade sessions
	if cm := roomManager.GetCascadeManager(); cm != nil {
		cm.Close()
	}

	// Gracefully close Redis Broker & Client connection
	if b := roomManager.GetBroker(); b != nil {
		_ = b.Close()
	}
	if redisClient != nil {
		_ = redisClient.Close()
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
