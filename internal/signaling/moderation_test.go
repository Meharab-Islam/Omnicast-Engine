package signaling

import (
	"testing"
	"time"

	"omnicast/internal/models"
)

func TestRoom_BannedUser(t *testing.T) {
	room := models.NewRoom("room-mod-1", "host-1")

	if room.IsUserBanned("bad-user") {
		t.Fatalf("Expected user not to be banned initially")
	}

	room.AddBannedUser("bad-user")
	if !room.IsUserBanned("bad-user") {
		t.Fatalf("Expected user 'bad-user' to be banned")
	}
}

func TestRoom_ParticipantReconnectTimer(t *testing.T) {
	room := models.NewRoom("room-reconn-1", "host-1")
	userID := "viewer-weak-wifi"

	expired := false
	room.StartParticipantReconnectTimer(userID, 50*time.Millisecond, func() {
		expired = true
	})

	// Cancel before expiration
	stopped := room.CancelParticipantReconnectTimer(userID)
	if !stopped {
		t.Fatalf("Expected timer to be stopped")
	}

	time.Sleep(70 * time.Millisecond)
	if expired {
		t.Fatalf("Expected timer callback not to be triggered after cancellation")
	}
}

func TestRoom_EmptyRoomTimer(t *testing.T) {
	room := models.NewRoom("room-empty-1", "host-1")

	deleted := false
	room.StartEmptyRoomTimer(50*time.Millisecond, func() {
		deleted = true
	})

	stopped := room.CancelEmptyRoomTimer()
	if !stopped {
		t.Fatalf("Expected empty room timer to be stopped")
	}

	time.Sleep(70 * time.Millisecond)
	if deleted {
		t.Fatalf("Expected empty room callback not to fire after cancellation")
	}
}
