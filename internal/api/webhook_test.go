package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWebhookClient_BearerAuthAndWorkerPool(t *testing.T) {
	secret := "my_super_secret_webhook_key_2026"
	receivedEvents := make([]WebhookEvent, 0)
	var mu sync.Mutex

	// 1. Setup Mock Webhook HTTP Server verifying Bearer token & Signature
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Failed to read webhook request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// Verify Authorization: Bearer <secret>
		authHeader := r.Header.Get("Authorization")
		expectedAuth := "Bearer " + secret
		if authHeader != expectedAuth {
			t.Errorf("Invalid Authorization header: got '%s', expected '%s'", authHeader, expectedAuth)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Verify X-Signature
		sig := r.Header.Get("X-Signature")
		if !VerifySignature(body, sig, secret) {
			t.Errorf("Invalid webhook signature: %s", sig)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var event WebhookEvent
		if err := json.Unmarshal(body, &event); err != nil {
			t.Errorf("Failed to unmarshal webhook payload: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		mu.Lock()
		receivedEvents = append(receivedEvents, event)
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// 2. Initialize WebhookClient with 4 workers
	client := NewWebhookClient(ts.URL, secret, 4)
	client.Start()
	defer client.Stop()

	// 3. Dispatch events
	eventsToSend := []WebhookEvent{
		{EventType: EventRoomStarted, RoomID: "room-101", UserID: "host-1", Timestamp: time.Now().Unix()},
		{EventType: EventParticipantJoined, RoomID: "room-101", UserID: "viewer-1", Timestamp: time.Now().Unix()},
		{EventType: EventGiftSent, RoomID: "room-101", UserID: "viewer-1", Data: map[string]any{"coins": 50}, Timestamp: time.Now().Unix()},
		{EventType: EventParticipantLeft, RoomID: "room-101", UserID: "viewer-1", Timestamp: time.Now().Unix()},
		{EventType: EventRoomEnded, RoomID: "room-101", UserID: "host-1", Timestamp: time.Now().Unix()},
	}

	for _, e := range eventsToSend {
		client.Dispatch(e)
	}

	// 4. Wait for worker pool to flush
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	count := len(receivedEvents)
	mu.Unlock()

	if count != len(eventsToSend) {
		t.Fatalf("Expected %d webhook events, received %d", len(eventsToSend), count)
	}

	// Verify standard JSON payload format (event_type, room_id, user_id, timestamp)
	for _, received := range receivedEvents {
		if received.EventType == "" {
			t.Errorf("Expected non-empty event_type in payload")
		}
		if !strings.HasPrefix(received.RoomID, "room-") {
			t.Errorf("Unexpected room_id '%s'", received.RoomID)
		}
		if received.Timestamp <= 0 {
			t.Errorf("Expected positive unix timestamp, got %d", received.Timestamp)
		}
	}
}
