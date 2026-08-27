package broker

import (
	"testing"
)

func TestNewRedisBroker_EmptyAddr(t *testing.T) {
	b, err := NewRedisBroker("", "", 0)
	if err == nil {
		t.Fatal("expected error for empty redis address, got nil")
	}
	if b != nil {
		t.Fatal("expected nil broker for empty redis address")
	}
	if b.IsActive() {
		t.Fatal("expected IsActive() to be false for nil broker")
	}
}

func TestRedisBroker_NilOperations(t *testing.T) {
	var b *RedisBroker

	if b.IsActive() {
		t.Fatal("expected IsActive() to be false for nil broker")
	}

	err := b.PublishRoomEvent("room-101", nil)
	if err == nil {
		t.Fatal("expected error when publishing on nil broker")
	}

	err = b.SubscribeRoom("room-101")
	if err == nil {
		t.Fatal("expected error when subscribing on nil broker")
	}

	b.UnsubscribeRoom("room-101")
	err = b.Close()
	if err != nil {
		t.Fatalf("expected nil error on Close() of nil broker, got %v", err)
	}

	err = b.RegisterRoomOrigin("room-101", "ws://127.0.0.1:8080/ws", 0)
	if err == nil {
		t.Fatal("expected error on RegisterRoomOrigin of nil broker")
	}

	err = b.RemoveRoomOrigin("room-101")
	if err == nil {
		t.Fatal("expected error on RemoveRoomOrigin of nil broker")
	}

	err = b.SaveRoomState(nil, nil)
	if err == nil {
		t.Fatal("expected error on SaveRoomState of nil broker")
	}

	_, err = b.GetRoomState(nil, "room-101")
	if err == nil {
		t.Fatal("expected error on GetRoomState of nil broker")
	}

	_, err = b.IncrementHostScore(nil, "room-101", 10)
	if err == nil {
		t.Fatal("expected error on IncrementHostScore of nil broker")
	}

	err = b.DeleteRoomState(nil, "room-101")
	if err == nil {
		t.Fatal("expected error on DeleteRoomState of nil broker")
	}

	err = b.PushChatMessage(nil, "room-101", nil)
	if err != nil {
		t.Fatal("expected nil error on PushChatMessage of nil broker")
	}

	err = b.RefreshRoomTTL(nil, "room-101")
	if err != nil {
		t.Fatal("expected nil error on RefreshRoomTTL of nil broker")
	}

	err = b.BatchRefreshRoomTTLs(nil, []string{"room-101"})
	if err != nil {
		t.Fatal("expected nil error on BatchRefreshRoomTTLs of nil broker")
	}
}
