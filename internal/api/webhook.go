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
	EventRoomStarted WebhookEventType = "room_started"
	EventRoomEnded   WebhookEventType = "room_ended"
	EventUserJoined  WebhookEventType = "user_joined"
	EventUserLeft    WebhookEventType = "user_left"
	EventGiftSent    WebhookEventType = "gift_sent"
)

// WebhookEvent represents the structured JSON payload sent to WEBHOOK_URL
type WebhookEvent struct {
	Event     WebhookEventType `json:"event"`
	RoomID    string           `json:"room_id"`
	UserID    string           `json:"user_id,omitempty"`
	Timestamp int64            `json:"timestamp"`
	Data      any              `json:"data,omitempty"`
}

// WebhookDispatcher manages background event queueing and HTTP POST dispatching
type WebhookDispatcher struct {
	WebhookURL    string
	WebhookSecret string
	eventQueue    chan WebhookEvent
	httpClient    *http.Client
	stopChan      chan struct{}
	wg            sync.WaitGroup
	mu            sync.RWMutex
	running       bool
}

// NewWebhookDispatcher initializes a WebhookDispatcher using env vars WEBHOOK_URL and WEBHOOK_SECRET
func NewWebhookDispatcher() *WebhookDispatcher {
	webhookURL := os.Getenv("WEBHOOK_URL")
	webhookSecret := os.Getenv("WEBHOOK_SECRET")

	return &WebhookDispatcher{
		WebhookURL:    webhookURL,
		WebhookSecret: webhookSecret,
		eventQueue:    make(chan WebhookEvent, 1024),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		stopChan: make(chan struct{}),
	}
}

// Start launches the background worker goroutine
func (d *WebhookDispatcher) Start() {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return
	}
	d.running = true
	d.mu.Unlock()

	d.wg.Add(1)
	go d.worker()

	if d.WebhookURL != "" {
		log.Printf("[Webhook] Dispatcher started -> Target URL: %s\n", d.WebhookURL)
	} else {
		log.Println("[Webhook] Dispatcher started in idle mode (WEBHOOK_URL not configured).")
	}
}

// Stop gracefully stops the dispatcher and waits for pending events to flush
func (d *WebhookDispatcher) Stop() {
	d.mu.Lock()
	if !d.running {
		d.mu.Unlock()
		return
	}
	d.running = false
	close(d.stopChan)
	d.mu.Unlock()

	d.wg.Wait()
	log.Println("[Webhook] Dispatcher stopped gracefully.")
}

// Dispatch queues a WebhookEvent non-blockingly into the dispatcher channel
func (d *WebhookDispatcher) Dispatch(event WebhookEvent) {
	if event.Timestamp == 0 {
		event.Timestamp = time.Now().UTC().Unix()
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.running {
		return
	}

	select {
	case d.eventQueue <- event:
	default:
		log.Printf("[Webhook Warning] Event queue full. Dropped event: %s for Room %s\n", event.Event, event.RoomID)
	}
}

// worker listens on eventQueue and executes HTTP POST requests
func (d *WebhookDispatcher) worker() {
	defer d.wg.Done()

	for {
		select {
		case event := <-d.eventQueue:
			d.sendWebhook(event)

		case <-d.stopChan:
			// Drain remaining events in channel before exit
			for len(d.eventQueue) > 0 {
				event := <-d.eventQueue
				d.sendWebhook(event)
			}
			return
		}
	}
}

// sendWebhook serializes payload, calculates HMAC SHA-256 signature, and performs HTTP POST
func (d *WebhookDispatcher) sendWebhook(event WebhookEvent) {
	d.mu.RLock()
	targetURL := d.WebhookURL
	secret := d.WebhookSecret
	d.mu.RUnlock()

	if targetURL == "" {
		// WEBHOOK_URL is not set; skip sending
		return
	}

	jsonBytes, err := json.Marshal(event)
	if err != nil {
		log.Printf("[Webhook Error] Failed to marshal event %s: %v\n", event.Event, err)
		return
	}

	signature := GenerateSignature(jsonBytes, secret)

	req, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		log.Printf("[Webhook Error] Failed to create HTTP request: %v\n", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", signature)
	req.Header.Set("X-Webhook-Event", string(event.Event))
	req.Header.Set("User-Agent", "Go-Live-Media-Server-Webhook/1.0")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		log.Printf("[Webhook Error] Failed to send event '%s' to %s: %v\n", event.Event, targetURL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("[Webhook Success] Event '%s' delivered to %s (Status: %d)\n", event.Event, targetURL, resp.StatusCode)
	} else {
		log.Printf("[Webhook Warning] Event '%s' delivered with non-2xx status (%d) from %s\n", event.Event, resp.StatusCode, targetURL)
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
