package metrics

import (
	"testing"
)

func TestMetricsTracking(t *testing.T) {
	IncActiveRooms()
	DecActiveRooms()

	IncActiveParticipants()
	DecActiveParticipants()

	AddBytesReceived(1024)
	AddBytesSent(2048)

	summary := GetSystemSummary()
	if summary.NumCPU <= 0 {
		t.Errorf("Expected NumCPU > 0, got %d", summary.NumCPU)
	}
	if summary.NumGoroutine <= 0 {
		t.Errorf("Expected NumGoroutine > 0, got %d", summary.NumGoroutine)
	}
}
