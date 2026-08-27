package webrtc

import (
	"sync"
)

// Simulcast layer constants
const (
	LayerHigh   = "f" // Full (High quality: 1080p/720p @ ~1.2 Mbps)
	LayerMedium = "h" // Half (Medium quality: 480p/360p @ ~500 kbps)
	LayerLow    = "q" // Quarter (Low quality: 240p/180p @ ~150 kbps)
)

// ABRController determines the optimal simulcast video layer based on estimated bandwidth and packet loss
type ABRController struct {
	mu sync.RWMutex
}

// NewABRController initializes a new Adaptive Bitrate Controller
func NewABRController() *ABRController {
	return &ABRController{}
}

// EvaluateLayer selects the optimal layer RID ('f', 'h', 'q') based on estimated bandwidth and packet loss percentage
func (a *ABRController) EvaluateLayer(bitrateBps uint64, lossPct float64) string {
	// Severe packet loss or very low bandwidth -> Downgrade to Low ('q')
	if lossPct >= 5.0 || bitrateBps < 300000 {
		return LayerLow
	}

	// Good bandwidth and low loss -> Upgrade to High ('f')
	if bitrateBps >= 800000 && lossPct < 2.0 {
		return LayerHigh
	}

	// Default to Medium ('h')
	return LayerMedium
}

// DynacastEngine manages layer subscriber tracking across rooms to save host upstream bandwidth
type DynacastEngine struct {
	roomSubscribers map[string]map[string]int // roomID -> rid -> count
	mu              sync.RWMutex
}

// NewDynacastEngine creates and initializes a new DynacastEngine
func NewDynacastEngine() *DynacastEngine {
	return &DynacastEngine{
		roomSubscribers: make(map[string]map[string]int),
	}
}

// AddSubscriber registers a subscriber for a specific layer in a room.
// Returns true if this is the first subscriber for the layer (meaning the host should resume this layer).
func (d *DynacastEngine) AddSubscriber(roomID, rid string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if rid == "" {
		rid = LayerMedium
	}

	layers, exists := d.roomSubscribers[roomID]
	if !exists {
		layers = make(map[string]int)
		d.roomSubscribers[roomID] = layers
	}

	oldCount := layers[rid]
	layers[rid] = oldCount + 1

	// If count was 0 and is now 1, signal to resume
	return oldCount == 0
}

// RemoveSubscriber unregisters a subscriber from a layer in a room.
// Returns true if the subscriber count dropped to 0 (meaning the host can pause this layer).
func (d *DynacastEngine) RemoveSubscriber(roomID, rid string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if rid == "" {
		rid = LayerMedium
	}

	layers, exists := d.roomSubscribers[roomID]
	if !exists {
		return false
	}

	oldCount := layers[rid]
	if oldCount <= 1 {
		delete(layers, rid)
		return true // Dropped to 0 -> Pause layer
	}

	layers[rid] = oldCount - 1
	return false
}

// SwitchSubscriber updates subscriber layer from oldRID to newRID atomically.
// Returns (shouldResumeNewLayer, shouldPauseOldLayer)
func (d *DynacastEngine) SwitchSubscriber(roomID, oldRID, newRID string) (bool, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if oldRID == newRID {
		return false, false
	}

	layers, exists := d.roomSubscribers[roomID]
	if !exists {
		layers = make(map[string]int)
		d.roomSubscribers[roomID] = layers
	}

	// Decrement old
	shouldPauseOld := false
	if oldRID != "" {
		oldCount := layers[oldRID]
		if oldCount <= 1 {
			delete(layers, oldRID)
			shouldPauseOld = true
		} else {
			layers[oldRID] = oldCount - 1
		}
	}

	// Increment new
	newCount := layers[newRID]
	layers[newRID] = newCount + 1
	shouldResumeNew := (newCount == 0)

	return shouldResumeNew, shouldPauseOld
}

// GetLayerCounts returns a snapshot copy of subscriber counts for a room
func (d *DynacastEngine) GetLayerCounts(roomID string) map[string]int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make(map[string]int)
	if layers, exists := d.roomSubscribers[roomID]; exists {
		for k, v := range layers {
			result[k] = v
		}
	}
	return result
}

// RemoveRoom cleans up all subscriber records when a room closes
func (d *DynacastEngine) RemoveRoom(roomID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.roomSubscribers, roomID)
}
