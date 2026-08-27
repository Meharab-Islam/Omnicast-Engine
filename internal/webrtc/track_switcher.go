package webrtc

import (
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

// TrackSwitcher intercepts incoming simulcast RTP packets and smoothly relays them to a viewer
// by rewriting RTP Sequence Numbers and Timestamps to ensure uninterrupted video decoding during layer switches.
type TrackSwitcher struct {
	outTrack        *webrtc.TrackLocalStaticRTP
	currentLayer    string
	targetLayer     string
	waitingKeyframe bool

	lastInSeq  uint16
	lastInTS   uint32
	lastOutSeq uint16
	lastOutTS  uint32

	seqOffset uint16
	tsOffset  uint32

	started bool
	mu      sync.RWMutex
}

// NewTrackSwitcher creates and initializes a new TrackSwitcher with a default layer (e.g., 'h' or 'f')
func NewTrackSwitcher(outTrack *webrtc.TrackLocalStaticRTP, initialLayer string) *TrackSwitcher {
	if initialLayer == "" {
		initialLayer = "h"
	}
	return &TrackSwitcher{
		outTrack:     outTrack,
		currentLayer: initialLayer,
		targetLayer:  initialLayer,
	}
}

// GetCurrentLayer returns the currently active simulcast layer RID ('q', 'h', 'f')
func (ts *TrackSwitcher) GetCurrentLayer() string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.currentLayer
}

// GetTargetLayer returns the target layer pending switch
func (ts *TrackSwitcher) GetTargetLayer() string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.targetLayer
}

// SwitchLayer requests a switch to a target simulcast layer RID.
// The switcher will transition cleanly on the next keyframe.
func (ts *TrackSwitcher) SwitchLayer(targetRID string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.currentLayer == targetRID {
		ts.targetLayer = targetRID
		ts.waitingKeyframe = false
		return
	}

	ts.targetLayer = targetRID
	ts.waitingKeyframe = true
}

// ForceSwitchLayer switches the layer immediately
func (ts *TrackSwitcher) ForceSwitchLayer(targetRID string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.currentLayer = targetRID
	ts.targetLayer = targetRID
	ts.waitingKeyframe = false
}

// WriteRTP processes an incoming RTP packet from a specific RID, rewrites its SequenceNumber & Timestamp,
// and writes it to the outgoing TrackLocalStaticRTP.
func (ts *TrackSwitcher) WriteRTP(rid string, packet *rtp.Packet) error {
	if packet == nil || ts.outTrack == nil {
		return nil
	}

	ts.mu.Lock()
	defer ts.mu.Unlock()

	// Check if this packet is from the target layer during a pending layer switch
	if ts.waitingKeyframe && rid == ts.targetLayer {
		// Switch to target layer
		ts.currentLayer = ts.targetLayer
		ts.waitingKeyframe = false

		// Calculate offsets to maintain continuous sequence numbers & timestamps
		if ts.started {
			ts.seqOffset = (ts.lastOutSeq + 1) - packet.SequenceNumber
			ts.tsOffset = (ts.lastOutTS + 3000) - packet.Timestamp
		}
	}

	// Drop packets that do not belong to the currently active layer
	if rid != ts.currentLayer {
		return nil
	}

	// Clone packet for zero-mutation safety
	outPkt := *packet

	if !ts.started {
		// Initialize starting point
		ts.started = true
		ts.lastInSeq = packet.SequenceNumber
		ts.lastInTS = packet.Timestamp
		ts.lastOutSeq = packet.SequenceNumber
		ts.lastOutTS = packet.Timestamp
		ts.seqOffset = 0
		ts.tsOffset = 0
	} else {
		// Rewrite Sequence Number & Timestamp
		outPkt.SequenceNumber = packet.SequenceNumber + ts.seqOffset
		outPkt.Timestamp = packet.Timestamp + ts.tsOffset

		ts.lastInSeq = packet.SequenceNumber
		ts.lastInTS = packet.Timestamp
		ts.lastOutSeq = outPkt.SequenceNumber
		ts.lastOutTS = outPkt.Timestamp
	}

	return ts.outTrack.WriteRTP(&outPkt)
}

// GetOutputTrack returns the underlying egress TrackLocalStaticRTP
func (ts *TrackSwitcher) GetOutputTrack() *webrtc.TrackLocalStaticRTP {
	return ts.outTrack
}
