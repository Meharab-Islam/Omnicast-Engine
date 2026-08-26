package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestWebhookDispatcher(t *testing.T) {
	secret := "my_super_secret_webhook_key_2026"
	receivedEvents := make([]WebhookEvent, 0)
	var mu sync.Mutex

	// 1. Setup Mock Webhook HTTP Server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Failed to read webhook request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// Verify signature
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

	// 2. Initialize Dispatcher with Mock Server URL
	dispatcher := NewWebhookDispatcher()
	dispatcher.WebhookURL = ts.URL
	dispatcher.WebhookSecret = secret
	dispatcher.Start()
	defer dispatcher.Stop()

	// 3. Dispatch all 5 event types
	eventsToSend := []WebhookEvent{
		{Event: EventRoomStarted, RoomID: "room-101", UserID: "host-1", Timestamp: time.Now().Unix()},
		{Event: EventUserJoined, RoomID: "room-101", UserID: "viewer-1", Timestamp: time.Now().Unix()},
		{Event: EventGiftSent, RoomID: "room-101", UserID: "viewer-1", Data: map[string]any{"coins": 50}, Timestamp: time.Now().Unix()},
		{Event: EventUserLeft, RoomID: "room-101", UserID: "viewer-1", Timestamp: time.Now().Unix()},
		{Event: EventRoomEnded, RoomID: "room-101", UserID: "host-1", Timestamp: time.Now().Unix()},
	}

	for _, e := range eventsToSend {
		dispatcher.Dispatch(e)
	}

	// 4. Wait for worker to flush events
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	count := len(receivedEvents)
	mu.Unlock()

	if count != len(eventsToSend) {
		t.Fatalf("Expected %d webhook events, received %d", len(eventsToSend), count)
	}

	for i, expected := range eventsToSend {
		if receivedEvents[i].Event != expected.Event {
			t.Errorf("Event %d: expected '%s', got '%s'", i, expected.Event, receivedEvents[i].Event)
		}
		if receivedEvents[i].RoomID != expected.RoomID {
			t.Errorf("Event %d: expected RoomID '%s', got '%s'", i, expected.RoomID, receivedEvents[i].RoomID)
		}
	}
}
