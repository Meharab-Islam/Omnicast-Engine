package webrtc

import (
	"log"
	"sync"
)

// ViewportManager manages dynamic viewports and server-side track pausing for thousands of viewers.
// When a viewer's app notifies the SFU which co-hosts are currently visible on their screen
// (e.g., via WebSocket "set_viewport" action), ViewportManager stores the whitelist.
// RTP packets for off-screen/paused co-hosts are immediately dropped before encoding or transmission,
// saving massive bandwidth and client battery while keeping the WebRTC PeerConnection alive.
type ViewportManager struct {
	mu        sync.RWMutex
	viewports map[string]map[string]bool // viewerID -> set of visible coHostIDs/trackIDs
	roomID    string
}

// NewViewportManager creates a new ViewportManager for a Room.
func NewViewportManager(roomID string) *ViewportManager {
	return &ViewportManager{
		viewports: make(map[string]map[string]bool),
		roomID:    roomID,
	}
}

// SetVisibleTracks updates the list of co-hosts/tracks that a specific viewer is currently viewing.
// Passing an empty list resets the viewer to default behavior (all tracks visible).
func (vm *ViewportManager) SetVisibleTracks(viewerID string, visibleCoHosts []string) {
	if viewerID == "" {
		return
	}

	vm.mu.Lock()
	defer vm.mu.Unlock()

	if len(visibleCoHosts) == 0 {
		// Reset to default (all tracks visible)
		delete(vm.viewports, viewerID)
		log.Printf("[Dynamic Viewport] Room %s: Viewer %s reset viewport (all tracks visible)\n",
			vm.roomID, viewerID)
		return
	}

	trackSet := make(map[string]bool, len(visibleCoHosts))
	for _, id := range visibleCoHosts {
		if id != "" {
			trackSet[id] = true
		}
	}
	vm.viewports[viewerID] = trackSet

	log.Printf("[Dynamic Viewport] Room %s: Viewer %s updated viewport to %v (other tracks paused server-side)\n",
		vm.roomID, viewerID, visibleCoHosts)
}

// IsTrackVisible checks whether a specific co-host track should be forwarded to the given viewer.
// Returns true if:
// 1. The track is the Main Host ("host" or empty ID — always visible).
// 2. The viewer has not restricted their viewport (default = all visible).
// 3. The co-host ID is explicitly present in the viewer's visible whitelist.
func (vm *ViewportManager) IsTrackVisible(viewerID string, coHostID string) bool {
	// Main host is always visible
	if coHostID == "" || coHostID == "host" || coHostID == "main" {
		return true
	}

	vm.mu.RLock()
	defer vm.mu.RUnlock()

	visibleSet, exists := vm.viewports[viewerID]
	if !exists {
		// No viewport restriction set: all tracks are visible by default
		return true
	}

	return visibleSet[coHostID]
}

// GetVisibleTracks returns a slice of currently visible co-host IDs for a given viewer.
// Returns nil if no restriction is set (all visible).
func (vm *ViewportManager) GetVisibleTracks(viewerID string) []string {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	visibleSet, exists := vm.viewports[viewerID]
	if !exists {
		return nil
	}

	var tracks []string
	for id := range visibleSet {
		tracks = append(tracks, id)
	}
	return tracks
}

// RemoveViewer cleans up viewport state when a viewer leaves the room.
func (vm *ViewportManager) RemoveViewer(viewerID string) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	delete(vm.viewports, viewerID)
}

// GetActiveViewportCount returns the number of viewers currently using dynamic viewport pausing.
func (vm *ViewportManager) GetActiveViewportCount() int {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	return len(vm.viewports)
}
