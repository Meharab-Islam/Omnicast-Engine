package models

import (
	"encoding/json"
	"testing"
)

func TestParseSignalingMessage(t *testing.T) {
	// Valid message test
	raw := []byte(`{"event":"offer","room_id":"room-1","user_id":"user-1","payload":{"sdp":"v=0..."}}`)
	msg, err := ParseSignalingMessage(raw)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if msg.Event != EventOffer {
		t.Errorf("Expected event %s, got %s", EventOffer, msg.Event)
	}
	if msg.RoomID != "room-1" {
		t.Errorf("Expected room_id 'room-1', got %s", msg.RoomID)
	}

	// Empty data test
	_, err = ParseSignalingMessage([]byte(""))
	if err == nil {
		t.Error("Expected error for empty data, got nil")
	}

	// Missing event field test
	_, err = ParseSignalingMessage([]byte(`{"room_id":"room-1"}`))
	if err == nil {
		t.Error("Expected error for missing event, got nil")
	}
}

func TestSignalingMessageEncode(t *testing.T) {
	msg := &SignalingMessage{
		Event:   EventAnswer,
		RoomID:  "room-1",
		UserID:  "user-1",
		Payload: json.RawMessage(`{"sdp":"answer..."}`),
	}

	bytes, err := msg.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	parsed, err := ParseSignalingMessage(bytes)
	if err != nil {
		t.Fatalf("Failed to parse encoded message: %v", err)
	}
	if parsed.Event != EventAnswer {
		t.Errorf("Expected event %s, got %s", EventAnswer, parsed.Event)
	}

	// Action format test
	actionRaw := []byte(`{"action":"publish","room_id":"room-101","payload":{"sdp":"v=0..."}}`)
	actionMsg, err := ParseSignalingMessage(actionRaw)
	if err != nil {
		t.Fatalf("Failed to parse action message: %v", err)
	}
	if actionMsg.Event != "publish" || actionMsg.Action != "publish" {
		t.Fatalf("Expected action/event 'publish', got %s / %s", actionMsg.Action, actionMsg.Event)
	}
}
