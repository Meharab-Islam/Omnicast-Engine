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

	_, err = b.GetRoomOrigin("room-101")
	if err == nil {
		t.Fatal("expected error on GetRoomOrigin of nil broker")
	}

	err = b.RemoveRoomOrigin("room-101")
	if err == nil {
		t.Fatal("expected error on RemoveRoomOrigin of nil broker")
	}
}
