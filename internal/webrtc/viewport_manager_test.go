package webrtc

import (
	"testing"
)

func TestViewportManager_DefaultsAndVisibility(t *testing.T) {
	vm := NewViewportManager("test_room")

	// Main host is always visible regardless of viewport setting
	if !vm.IsTrackVisible("viewer_1", "host") {
		t.Fatalf("expected host to be visible")
	}
	if !vm.IsTrackVisible("viewer_1", "") {
		t.Fatalf("expected empty track ID to be visible (main host)")
	}

	// By default, before setting viewport, all co-hosts are visible
	if !vm.IsTrackVisible("viewer_1", "cohost_1") {
		t.Fatalf("expected cohost_1 to be visible by default")
	}
	if !vm.IsTrackVisible("viewer_1", "cohost_2") {
		t.Fatalf("expected cohost_2 to be visible by default")
	}
}

func TestViewportManager_SetVisibleTracks(t *testing.T) {
	vm := NewViewportManager("test_room")

	// Viewer 1 only looks at cohost_1 and cohost_2
	vm.SetVisibleTracks("viewer_1", []string{"cohost_1", "cohost_2"})

	if !vm.IsTrackVisible("viewer_1", "cohost_1") {
		t.Fatalf("expected cohost_1 to be visible")
	}
	if !vm.IsTrackVisible("viewer_1", "cohost_2") {
		t.Fatalf("expected cohost_2 to be visible")
	}

	// Co-hosts 3..12 should be paused / invisible for viewer_1
	if vm.IsTrackVisible("viewer_1", "cohost_3") {
		t.Fatalf("expected cohost_3 to be PAUSED/invisible for viewer_1")
	}
	if vm.IsTrackVisible("viewer_1", "cohost_12") {
		t.Fatalf("expected cohost_12 to be PAUSED/invisible for viewer_1")
	}

	// Another viewer without viewport set should still see all
	if !vm.IsTrackVisible("viewer_2", "cohost_3") {
		t.Fatalf("expected viewer_2 to see cohost_3 by default")
	}

	if count := vm.GetActiveViewportCount(); count != 1 {
		t.Fatalf("expected 1 active viewport, got %d", count)
	}
}

func TestViewportManager_ResetAndRemove(t *testing.T) {
	vm := NewViewportManager("test_room")

	vm.SetVisibleTracks("viewer_1", []string{"cohost_1"})
	if vm.IsTrackVisible("viewer_1", "cohost_2") {
		t.Fatalf("expected cohost_2 to be paused")
	}

	// Reset with empty list
	vm.SetVisibleTracks("viewer_1", []string{})
	if !vm.IsTrackVisible("viewer_1", "cohost_2") {
		t.Fatalf("expected cohost_2 to be visible after reset")
	}

	// Set again and remove
	vm.SetVisibleTracks("viewer_1", []string{"cohost_1"})
	vm.RemoveViewer("viewer_1")
	if !vm.IsTrackVisible("viewer_1", "cohost_2") {
		t.Fatalf("expected cohost_2 to be visible after RemoveViewer")
	}
	if count := vm.GetActiveViewportCount(); count != 0 {
		t.Fatalf("expected 0 active viewports after remove, got %d", count)
	}
}
