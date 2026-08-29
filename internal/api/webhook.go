package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// WebhookEventType defines all supported webhook events
type WebhookEventType string

const (
	EventRoomStarted       WebhookEventType = "RoomStarted"
	EventRoomEnded         WebhookEventType = "RoomEnded"
	EventParticipantJoined WebhookEventType = "ParticipantJoined"
	EventParticipantLeft   WebhookEventType = "ParticipantLeft"
	EventGiftSent          WebhookEventType = "GiftSent"

	// Legacy snake_case event names for backwards compatibility
	EventRoomStartedLegacy WebhookEventType = "room_started"
	EventRoomEndedLegacy   WebhookEventType = "room_ended"
	EventUserJoinedLegacy  WebhookEventType = "user_joined"
	EventUserLeftLegacy    WebhookEventType = "user_left"
)

// WebhookEvent represents the standard structured JSON payload format for room events
type WebhookEvent struct {
	EventType WebhookEventType `json:"event_type"`
	Event     WebhookEventType `json:"event,omitempty"` // For backwards compatibility
	RoomID    string           `json:"room_id"`
	UserID    string           `json:"user_id,omitempty"`
	Timestamp int64            `json:"timestamp"`
	Data      any              `json:"data,omitempty"`
}

// WebhookClient manages background event queueing and HTTP POST dispatching with a dedicated worker pool
type WebhookClient struct {
	WebhookURL    string
	WebhookSecret string
	numWorkers    int
	eventQueue    chan WebhookEvent
	httpClient    *http.Client
	stopChan      chan struct{}
	wg            sync.WaitGroup
	mu            sync.RWMutex
	running       bool
}

// WebhookDispatcher is an alias for WebhookClient for existing call sites
type WebhookDispatcher = WebhookClient

// NewWebhookClient initializes a WebhookClient with an internal worker pool and a buffered Go channel
func NewWebhookClient(webhookURL, webhookSecret string, numWorkers int) *WebhookClient {
	if numWorkers <= 0 {
		numWorkers = 8
	}

	return &WebhookClient{
		WebhookURL:    webhookURL,
		WebhookSecret: webhookSecret,
		numWorkers:    numWorkers,
		eventQueue:    make(chan WebhookEvent, 4096),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		stopChan: make(chan struct{}),
	}
}

// NewWebhookDispatcher initializes a WebhookDispatcher using env vars WEBHOOK_URL and WEBHOOK_SECRET
func NewWebhookDispatcher() *WebhookDispatcher {
	webhookURL := os.Getenv("WEBHOOK_URL")
	webhookSecret := os.Getenv("WEBHOOK_SECRET")

	return NewWebhookClient(webhookURL, webhookSecret, 8)
}

// Start launches the background worker pool to dispatch webhooks without blocking WebRTC threads
func (c *WebhookClient) Start() {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.mu.Unlock()

	for i := 0; i < c.numWorkers; i++ {
		c.wg.Add(1)
		go c.worker(i)
	}

	if c.WebhookURL != "" {
		log.Printf("[Webhook] Dispatcher started with %d workers -> Target URL: %s\n", c.numWorkers, c.WebhookURL)
	} else {
		log.Printf("[Webhook] Dispatcher started in idle mode with %d workers (WEBHOOK_URL not configured).\n", c.numWorkers)
	}
}

// Stop gracefully stops the dispatcher worker pool and waits for pending events to flush
func (c *WebhookClient) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	c.running = false
	close(c.stopChan)
	c.mu.Unlock()

	c.wg.Wait()
	log.Println("[Webhook] Dispatcher stopped gracefully.")
}

// Dispatch queues a WebhookEvent non-blockingly into the dispatcher channel
func (c *WebhookClient) Dispatch(event WebhookEvent) {
	if event.Timestamp == 0 {
		event.Timestamp = time.Now().UTC().Unix()
	}
	if event.EventType == "" && event.Event != "" {
		event.EventType = event.Event
	}
	if event.Event == "" && event.EventType != "" {
		event.Event = event.EventType
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.running {
		return
	}

	select {
	case c.eventQueue <- event:
	default:
		log.Printf("[Webhook Warning] Event queue full. Dropped event: %s for Room %s\n", event.EventType, event.RoomID)
	}
}

// worker listens on eventQueue and executes HTTP POST requests concurrently
func (c *WebhookClient) worker(workerID int) {
	defer c.wg.Done()

	for {
		select {
		case event := <-c.eventQueue:
			c.sendWebhook(event)

		case <-c.stopChan:
			// Drain remaining events in channel before exit
			for len(c.eventQueue) > 0 {
				select {
				case event := <-c.eventQueue:
					c.sendWebhook(event)
				default:
					return
				}
			}
			return
		}
	}
}

// sendWebhook serializes payload, adds Authorization: Bearer header & X-Signature, and performs HTTP POST
func (c *WebhookClient) sendWebhook(event WebhookEvent) {
	c.mu.RLock()
	targetURL := c.WebhookURL
	secret := c.WebhookSecret
	c.mu.RUnlock()

	if targetURL == "" {
		return
	}

	jsonBytes, err := json.Marshal(event)
	if err != nil {
		log.Printf("[Webhook Error] Failed to marshal event %s: %v\n", event.EventType, err)
		return
	}

	signature := GenerateSignature(jsonBytes, secret)

	req, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		log.Printf("[Webhook Error] Failed to create HTTP request: %v\n", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	// Include Authorization: Bearer <secret> header so backend can verify origin
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	if signature != "" {
		req.Header.Set("X-Signature", signature)
	}
	req.Header.Set("X-Webhook-Event", string(event.EventType))
	req.Header.Set("User-Agent", "Omnicast-SFU-Webhook/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[Webhook Error] Failed to send event '%s' to %s: %v\n", event.EventType, targetURL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("[Webhook Success] Event '%s' delivered to %s (Status: %d)\n", event.EventType, targetURL, resp.StatusCode)
	} else {
		log.Printf("[Webhook Warning] Event '%s' delivered with non-2xx status (%d) from %s\n", event.EventType, resp.StatusCode, targetURL)
	}
}

// GenerateSignature generates HMAC-SHA256 hex signature from payload and secret
func GenerateSignature(payload []byte, secret string) string {
	if secret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature verifies if an incoming signature matches expected HMAC-SHA256 signature
func VerifySignature(payload []byte, signature, secret string) bool {
	if secret == "" && signature == "" {
		return true
	}
	expected := GenerateSignature(payload, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}
