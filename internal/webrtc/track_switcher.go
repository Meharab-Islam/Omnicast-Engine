package webrtc

import (
	"log"
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	"omnicast/internal/metrics"
)

// TrackSwitcher intercepts incoming video RTP packets and smoothly relays them to a viewer.
// It operates on a single VP9 video track with Scalable Video Coding (SVC L3T3)
// or seamlessly relays multi-layer streams by rewriting RTP Sequence Numbers and Timestamps.
type TrackSwitcher struct {
	// Single incoming VP9 video track for SVC operation
	inputTrack *webrtc.TrackRemote

	// Backward-compatible references for simulcast layers
	trackQ *webrtc.TrackRemote // Low Resolution (RID 'q')
	trackH *webrtc.TrackRemote // Medium Resolution (RID 'h')
	trackF *webrtc.TrackRemote // High / Full Resolution (RID 'f')

	// Active track pointer currently being forwarded
	activeTrack *webrtc.TrackRemote

	// Target track pointer to switch to
	targetTrack *webrtc.TrackRemote

	// Flag indicating whether a layer/track switch is pending
	pendingSwitch bool

	// One outgoing track for the Viewer
	outTrack *webrtc.TrackLocalStaticRTP

	currentLayer    string
	targetLayer     string
	waitingKeyframe bool

	// SVC (Scalable Video Coding) Spatial and Temporal Layer Controls
	currentSpatialLayer  uint8 // S: 0=Low ('q'), 1=Medium ('h'), 2=High ('f')
	targetSpatialLayer   uint8
	currentTemporalLayer uint8 // T: 0=7.5fps, 1=15fps, 2=30fps
	targetTemporalLayer  uint8
	vp9Parser            *VP9PayloadParser

	lastInSeq  uint16
	lastInTS   uint32
	lastOutSeq uint16
	lastOutTS  uint32

	seqOffset uint16
	tsOffset  uint32

	// Dedicated SequenceNumberAdjuster to rewrite RTP sequence numbers across layer switches
	seqAdjuster *SequenceNumberAdjuster

	// Dedicated TimestampAdjuster to rewrite RTP timestamps across layer switches
	tsAdjuster *TimestampAdjuster

	started             bool
	hasReceivedKeyframe bool
	queue               chan *rtp.Packet
	closed              chan struct{}
	closeOnce           sync.Once
	mu                  sync.RWMutex
}

// NewTrackSwitcher creates and initializes a new TrackSwitcher with a default layer (e.g., 'h' or 'f')
func NewTrackSwitcher(outTrack *webrtc.TrackLocalStaticRTP, initialLayer string) *TrackSwitcher {
	if initialLayer == "" {
		initialLayer = "h"
	}
	var initialSpatial uint8 = 1 // default 'h' (Medium)
	switch initialLayer {
	case LayerLow:
		initialSpatial = 0
	case LayerMedium:
		initialSpatial = 1
	case LayerHigh:
		initialSpatial = 2
	}

	ts := &TrackSwitcher{
		outTrack:             outTrack,
		currentLayer:         initialLayer,
		targetLayer:          initialLayer,
		currentSpatialLayer:  initialSpatial,
		targetSpatialLayer:   initialSpatial,
		currentTemporalLayer: 2, // Full 30fps
		targetTemporalLayer:  2,
		waitingKeyframe:      true,
		hasReceivedKeyframe:  false,
		vp9Parser:            NewVP9PayloadParser(),
		seqAdjuster:          NewSequenceNumberAdjuster(),
		tsAdjuster:           NewTimestampAdjuster(),
		queue:                make(chan *rtp.Packet, 256),
		closed:               make(chan struct{}),
	}

	// Dedicated background worker to write RTP packets asynchronously to the viewer's track
	// This completely protects the SFU broadcast loop from blocking on slow/lagging viewers
	go func() {
		for {
			select {
			case <-ts.closed:
				return
			case pkt, ok := <-ts.queue:
				if !ok {
					return
				}
				if ts.outTrack != nil {
					_ = ts.outTrack.WriteRTP(pkt)
					metrics.AddBytesSent(pkt.MarshalSize())
				}
			}
		}
	}()

	return ts
}

// NewVP9TrackSwitcher creates a TrackSwitcher operating on a single incoming VP9 video track with SVC L3T3
func NewVP9TrackSwitcher(inputTrack *webrtc.TrackRemote, outTrack *webrtc.TrackLocalStaticRTP, initialLayer string) *TrackSwitcher {
	ts := NewTrackSwitcher(outTrack, initialLayer)
	ts.inputTrack = inputTrack
	ts.activeTrack = inputTrack
	return ts
}

// SetInputTrack sets the single incoming VP9 video track for SVC operation
func (ts *TrackSwitcher) SetInputTrack(track *webrtc.TrackRemote) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.inputTrack = track
	ts.activeTrack = track
}

// GetInputTrack returns the single incoming VP9 video track
func (ts *TrackSwitcher) GetInputTrack() *webrtc.TrackRemote {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.inputTrack
}

// SwitchSVCLayers changes the target Spatial (S) and Temporal (T) layers for the single VP9 track
func (ts *TrackSwitcher) SwitchSVCLayers(targetSpatial, targetTemporal uint8) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if targetSpatial > 2 {
		targetSpatial = 2
	}
	if targetTemporal > 2 {
		targetTemporal = 2
	}

	ts.targetSpatialLayer = targetSpatial
	ts.targetTemporalLayer = targetTemporal

	switch targetSpatial {
	case 0:
		ts.targetLayer = LayerLow
	case 1:
		ts.targetLayer = LayerMedium
	case 2:
		ts.targetLayer = LayerHigh
	}

	if targetSpatial != ts.currentSpatialLayer || targetTemporal != ts.currentTemporalLayer {
		ts.pendingSwitch = true
		ts.waitingKeyframe = true
	} else {
		ts.pendingSwitch = false
		ts.waitingKeyframe = false
	}
}

// DropHighestSpatialLayer instructs the TrackSwitcher to drop the highest active Spatial Layer
// (e.g. drop S=2 down to S=1, only forwarding S=0 and S=1; or drop S=1 down to S=0) to alleviate network congestion.
// Returns the new target spatial layer.
func (ts *TrackSwitcher) DropHighestSpatialLayer() uint8 {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.currentSpatialLayer > 0 {
		newSpatial := ts.currentSpatialLayer - 1
		ts.targetSpatialLayer = newSpatial
		switch newSpatial {
		case 0:
			ts.targetLayer = LayerLow
		case 1:
			ts.targetLayer = LayerMedium
		}
		ts.pendingSwitch = true
		ts.waitingKeyframe = false // Down-switching can happen immediately
		ts.currentSpatialLayer = newSpatial
		ts.currentLayer = ts.targetLayer

		// Calculate offsets to maintain continuous sequence numbers & timestamps
		if ts.started {
			ts.seqOffset = ts.seqAdjuster.GetOffset()
		}

		log.Printf("[Congestion Control] Dropped highest spatial layer to S=%d (now forwarding layers S=0..S=%d, target layer: '%s')\n",
			newSpatial, newSpatial, ts.targetLayer)
		return newSpatial
	}
	return 0
}

// HandleCongestion checks packet loss and estimated bitrate from the TWCC Bandwidth Estimator:
// If congestion is detected (e.g., loss > 5% or bitrate < 1 Mbps), it instructs the TrackSwitcher
// to drop the highest spatial layer (e.g., drop S=2 and only forward S=0 and S=1).
func (ts *TrackSwitcher) HandleCongestion(packetLoss float64, bitrateBps int) bool {
	if packetLoss > LossThresholdHigh || (bitrateBps > 0 && bitrateBps < 1_000_000) {
		ts.mu.RLock()
		currentS := ts.currentSpatialLayer
		ts.mu.RUnlock()

		if currentS > 0 {
			ts.DropHighestSpatialLayer()
			return true
		}
	}
	return false
}

// DropHighestTemporalLayer instructs the TrackSwitcher to drop the highest active Temporal Layer
// (e.g. drop frames with T=2 while keeping and forwarding T=0 and T=1) when CPU or bandwidth is slightly constrained.
// Returns the new target temporal layer.
func (ts *TrackSwitcher) DropHighestTemporalLayer() uint8 {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.currentTemporalLayer > 0 {
		newTemporal := ts.currentTemporalLayer - 1
		ts.targetTemporalLayer = newTemporal
		ts.currentTemporalLayer = newTemporal

		log.Printf("[Temporal Layer Dropping] Dropped highest temporal layer to T=%d (dropping T=2, forwarding T=0 and T=1)\n",
			newTemporal)
		return newTemporal
	}
	return 0
}

// SetTemporalLayer sets the maximum allowed temporal layer (T=0, T=1, or T=2)
func (ts *TrackSwitcher) SetTemporalLayer(targetT uint8) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if targetT > 2 {
		targetT = 2
	}
	ts.targetTemporalLayer = targetT
	ts.currentTemporalLayer = targetT
	log.Printf("[Temporal Layer] Set maximum temporal layer to T=%d\n", targetT)
}

// HandleTemporalConstraint checks if CPU load or network bandwidth is slightly constrained:
// If so, it drops the highest temporal layer (dropping T=2 down to T=1, or T=1 down to T=0).
func (ts *TrackSwitcher) HandleTemporalConstraint(isConstrained bool) bool {
	if isConstrained {
		ts.mu.RLock()
		currentT := ts.currentTemporalLayer
		ts.mu.RUnlock()

		if currentT > 0 {
			ts.DropHighestTemporalLayer()
			return true
		}
	}
	return false
}

// GetSpatialLayer returns the currently active spatial layer (0, 1, or 2)
func (ts *TrackSwitcher) GetSpatialLayer() uint8 {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.currentSpatialLayer
}

// GetTemporalLayer returns the currently active temporal layer (0, 1, or 2)
func (ts *TrackSwitcher) GetTemporalLayer() uint8 {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.currentTemporalLayer
}

// NewSimulcastTrackSwitcher creates a TrackSwitcher with references to all 3 incoming Host tracks (RID 'q', 'h', 'f')
// and one outgoing webrtc.TrackLocalStaticRTP for the Viewer.
func NewSimulcastTrackSwitcher(trackQ, trackH, trackF *webrtc.TrackRemote, outTrack *webrtc.TrackLocalStaticRTP, initialLayer string) *TrackSwitcher {
	ts := NewTrackSwitcher(outTrack, initialLayer)
	ts.trackQ = trackQ
	ts.trackH = trackH
	ts.trackF = trackF
	return ts
}

// SetIncomingTracks updates the references to the 3 incoming Host tracks (RID 'q', 'h', 'f')
func (ts *TrackSwitcher) SetIncomingTracks(trackQ, trackH, trackF *webrtc.TrackRemote) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.trackQ = trackQ
	ts.trackH = trackH
	ts.trackF = trackF
}

// SetIncomingTrack sets a specific incoming Host track by RID ('q', 'h', 'f')
func (ts *TrackSwitcher) SetIncomingTrack(rid string, track *webrtc.TrackRemote) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	switch rid {
	case LayerLow:
		ts.trackQ = track
	case LayerMedium:
		ts.trackH = track
	case LayerHigh:
		ts.trackF = track
	}
}

// GetIncomingTrack retrieves the incoming Host track reference for a given RID
func (ts *TrackSwitcher) GetIncomingTrack(rid string) *webrtc.TrackRemote {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	switch rid {
	case LayerLow:
		return ts.trackQ
	case LayerMedium:
		return ts.trackH
	case LayerHigh:
		return ts.trackF
	default:
		return nil
	}
}

// GetOutTrack returns the outgoing webrtc.TrackLocalStaticRTP for the Viewer
func (ts *TrackSwitcher) GetOutTrack() *webrtc.TrackLocalStaticRTP {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.outTrack
}

// GetActiveTrack returns the active Host track pointer currently being forwarded to the viewer
func (ts *TrackSwitcher) GetActiveTrack() *webrtc.TrackRemote {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.activeTrack
}

// SetActiveTrack explicitly sets the active Host track pointer currently being forwarded
func (ts *TrackSwitcher) SetActiveTrack(track *webrtc.TrackRemote) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.activeTrack = track
}

// Close terminates the background worker goroutine
func (ts *TrackSwitcher) Close() {
	ts.closeOnce.Do(func() {
		close(ts.closed)
	})
}

// GetCurrentLayer returns the currently active layer RID ('q', 'h', 'f')
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

// GetTargetTrack returns the target track pending switch
func (ts *TrackSwitcher) GetTargetTrack() *webrtc.TrackRemote {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.targetTrack
}

// IsPendingSwitch returns whether a track switch is currently pending
func (ts *TrackSwitcher) IsPendingSwitch() bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.pendingSwitch
}

// SwitchTrack compares the target track with activeTrack:
// If they are different, sets pendingSwitch = true and stores the targetTrack.
// If they are the same, sets pendingSwitch = false and clears targetTrack.
func (ts *TrackSwitcher) SwitchTrack(targetTrack *webrtc.TrackRemote) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if targetTrack != ts.activeTrack {
		ts.pendingSwitch = true
		ts.targetTrack = targetTrack
		ts.waitingKeyframe = true
		if targetTrack != nil {
			ts.targetLayer = targetTrack.RID()
		}
	} else {
		ts.pendingSwitch = false
		ts.targetTrack = nil
		ts.waitingKeyframe = false
	}
}

// SetTargetTrack compares the target track with activeTrack and updates pendingSwitch
func (ts *TrackSwitcher) SetTargetTrack(targetTrack *webrtc.TrackRemote) {
	ts.SwitchTrack(targetTrack)
}

// SwitchLayer initiates switching to a target spatial layer ('q', 'h', 'f')
// In SVC mode, maps to spatial layers S0 (Low), S1 (Medium), S2 (High) on the single VP9 track.
func (ts *TrackSwitcher) SwitchLayer(targetRID string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	var targetSpatial uint8 = 1
	switch targetRID {
	case LayerLow:
		targetSpatial = 0
	case LayerMedium:
		targetSpatial = 1
	case LayerHigh:
		targetSpatial = 2
	}

	ts.targetLayer = targetRID
	ts.targetSpatialLayer = targetSpatial

	var targetTrack *webrtc.TrackRemote
	switch targetRID {
	case LayerLow:
		targetTrack = ts.trackQ
	case LayerMedium:
		targetTrack = ts.trackH
	case LayerHigh:
		targetTrack = ts.trackF
	}

	if targetSpatial != ts.currentSpatialLayer || targetRID != ts.currentLayer || (targetTrack != nil && targetTrack != ts.activeTrack) {
		ts.pendingSwitch = true
		ts.targetTrack = targetTrack
		ts.waitingKeyframe = true
	} else {
		ts.pendingSwitch = false
		ts.targetTrack = nil
		ts.waitingKeyframe = false
	}
}

// SwitchLayerByRID initiates switching to a target layer RID
func (ts *TrackSwitcher) SwitchLayerByRID(targetRID string) {
	ts.SwitchLayer(targetRID)
}

// ForceSwitchLayer switches the layer immediately
func (ts *TrackSwitcher) ForceSwitchLayer(targetRID string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.currentLayer = targetRID
	ts.targetLayer = targetRID
	switch targetRID {
	case LayerLow:
		ts.currentSpatialLayer = 0
		ts.targetSpatialLayer = 0
		ts.activeTrack = ts.trackQ
	case LayerMedium:
		ts.currentSpatialLayer = 1
		ts.targetSpatialLayer = 1
		ts.activeTrack = ts.trackH
	case LayerHigh:
		ts.currentSpatialLayer = 2
		ts.targetSpatialLayer = 2
		ts.activeTrack = ts.trackF
	}
	ts.pendingSwitch = false
	ts.waitingKeyframe = false
}

// WriteRTP processes an incoming RTP packet (from single VP9 track or simulcast layer),
// performs SVC spatial/temporal layer filtering, rewrites SequenceNumber & Timestamp,
// and pushes it to the non-blocking worker queue for asynchronous transmission to the viewer.
func (ts *TrackSwitcher) WriteRTP(rid string, packet *rtp.Packet) error {
	if packet == nil || ts.outTrack == nil {
		return nil
	}

	ts.mu.Lock()
	defer ts.mu.Unlock()

	// Check if closed
	select {
	case <-ts.closed:
		return nil
	default:
	}

	codecMime := ""
	if ts.outTrack != nil {
		codecMime = ts.outTrack.Codec().MimeType
	}

	// Check if payload is VP9
	isVP9 := codecMime == webrtc.MimeTypeVP9 || codecMime == "video/VP9"
	var vp9Desc *VP9PayloadDescriptor
	if isVP9 && len(packet.Payload) > 0 {
		vp9Desc, _ = ParseVP9Descriptor(packet.Payload)
	}

	// 1. Strict Initial Keyframe Gating for newly subscribed viewers
	// Discard/drop ALL incoming packets until a verified Keyframe (I-frame) arrives
	if !ts.hasReceivedKeyframe {
		isKey := false
		if isVP9 {
			isKey = (vp9Desc != nil && vp9Desc.IsKeyframe())
		} else {
			isKey = IsKeyframe(codecMime, packet.Payload)
		}

		if !isKey {
			// Discard delta frame (P-frame) for new viewer to prevent blocky square artifacts
			return nil
		}

		// Initial Keyframe arrived! Start forwarding cleanly from this keyframe
		ts.hasReceivedKeyframe = true
		ts.started = true
		ts.waitingKeyframe = false
		ts.pendingSwitch = false
		ts.lastInSeq = packet.SequenceNumber
		ts.lastInTS = packet.Timestamp
		ts.lastOutSeq = packet.SequenceNumber
		ts.lastOutTS = packet.Timestamp
		ts.seqOffset = 0
		ts.tsOffset = 0
		ts.seqAdjuster.NextContiguous(packet.SequenceNumber)
		ts.tsAdjuster.AdjustContinuous(packet.Timestamp, DefaultFrameDuration90kHz)

		outPkt := *packet
		outPkt.Header.SequenceNumber = ts.lastOutSeq
		outPkt.Header.Timestamp = ts.lastOutTS

		select {
		case ts.queue <- &outPkt:
		default:
		}
		return nil
	}

	if vp9Desc != nil {
		// Single VP9 Track with SVC (Scalable Video Coding) L3T3
		if ts.pendingSwitch || ts.waitingKeyframe {
			// Check if packet allows switching (Keyframe / Intra-frame or Up-switch point U=1)
			if vp9Desc.IsKeyframe() || vp9Desc.SwitchingUp {
				ts.currentSpatialLayer = ts.targetSpatialLayer
				ts.currentTemporalLayer = ts.targetTemporalLayer
				ts.currentLayer = ts.targetLayer
				ts.pendingSwitch = false
				ts.waitingKeyframe = false

				// Calculate offsets to maintain continuous sequence numbers & timestamps
				ts.seqAdjuster.Switch(packet.SequenceNumber)
				ts.seqOffset = ts.seqAdjuster.GetOffset()

				ts.tsAdjuster.Switch(packet.Timestamp, DefaultFrameDuration90kHz)
				ts.tsOffset = ts.tsAdjuster.GetOffset()
			} else if vp9Desc.S > ts.currentSpatialLayer {
				// While waiting for Keyframe/Up-switch point, drop higher spatial layer packets
				return nil
			}
		}

		// Filter out packets belonging to spatial (S) or temporal (T) layers above target
		if vp9Desc.HasLayerIndices {
			if vp9Desc.S > ts.currentSpatialLayer || vp9Desc.T > ts.currentTemporalLayer {
				return nil // Drop higher layer packet
			}
		}
	} else {
		// Multi-track simulcast (e.g. VP8 / H.264 streams)
		if ts.pendingSwitch || ts.waitingKeyframe {
			if rid == ts.targetLayer || (ts.targetTrack != nil && rid == ts.targetTrack.RID()) {
				isKey := IsKeyframe(codecMime, packet.Payload)
				if isKey {
					ts.currentLayer = ts.targetLayer
					if ts.targetTrack == nil {
						switch ts.targetLayer {
						case LayerLow:
							ts.targetTrack = ts.trackQ
						case LayerMedium:
							ts.targetTrack = ts.trackH
						case LayerHigh:
							ts.targetTrack = ts.trackF
						}
					}
					if ts.targetTrack != nil {
						ts.activeTrack = ts.targetTrack
						ts.targetTrack = nil
					}
					ts.pendingSwitch = false
					ts.waitingKeyframe = false

					ts.seqAdjuster.Switch(packet.SequenceNumber)
					ts.seqOffset = ts.seqAdjuster.GetOffset()

					ts.tsAdjuster.Switch(packet.Timestamp, DefaultFrameDuration90kHz)
					ts.tsOffset = ts.tsAdjuster.GetOffset()
				} else {
					return nil
				}
			}
		}

		if rid != ts.currentLayer && rid != "" && rid != "default" {
			return nil
		}
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
		ts.seqAdjuster.NextContiguous(packet.SequenceNumber)
		ts.tsAdjuster.AdjustContinuous(packet.Timestamp, DefaultFrameDuration90kHz)
		outPkt.Header.SequenceNumber = ts.lastOutSeq
		outPkt.Header.Timestamp = ts.lastOutTS
	} else {
		// Ensure that when SVC layers are dropped, the RTP Sequence Numbers and Timestamps
		// are rewritten contiguously (+1 for every forwarded packet, unbroken timestamp clock)
		// so the Viewer's decoder does not freeze, stall, or trigger false packet loss NACKs.
		if isVP9 {
			outPkt.Header.SequenceNumber = ts.seqAdjuster.NextContiguous(packet.SequenceNumber)
			outPkt.Header.Timestamp = ts.tsAdjuster.AdjustContinuous(packet.Timestamp, DefaultFrameDuration90kHz)
		} else {
			outPkt.Header.SequenceNumber = ts.seqAdjuster.Adjust(packet.SequenceNumber)
			outPkt.Header.Timestamp = ts.tsAdjuster.Adjust(packet.Timestamp)
		}

		ts.lastInSeq = packet.SequenceNumber
		ts.lastInTS = packet.Timestamp
		ts.lastOutSeq = outPkt.Header.SequenceNumber
		ts.lastOutTS = outPkt.Header.Timestamp
	}

	// Enqueue the rewritten packet for the background worker to call outTrack.WriteRTP()
	select {
	case ts.queue <- &outPkt:
	default:
		// Queue full: packet dropped for slow client to protect room broadcast loop
	}

	return nil
}

// GetOutputTrack returns the underlying egress TrackLocalStaticRTP
func (ts *TrackSwitcher) GetOutputTrack() *webrtc.TrackLocalStaticRTP {
	return ts.outTrack
}

// IsVP8Keyframe reads the VP8 payload descriptor and uncompressed frame header to detect if it is a Keyframe (I-frame).
// Per RFC 7741 Section 4.2:
// - It inspects the first byte of the VP8 payload (Descriptor) to check for extended control bits (X bit).
// - It advances to the first byte of the VP8 uncompressed frame header.
// - Bit 0 (P bit / inverse keyframe flag): 0 indicates a Keyframe (I-frame), 1 indicates an Inter-frame (P-frame).
func IsVP8Keyframe(payload []byte) bool {
	if len(payload) == 0 {
		return false
	}

	payloadIndex := 0
	// First byte: VP8 Payload Descriptor
	xBit := (payload[0] & 0x80) != 0
	payloadIndex++

	// If X bit is set, process optional extension bytes
	if xBit {
		if payloadIndex >= len(payload) {
			return false
		}
		ext := payload[payloadIndex]
		iBit := (ext & 0x80) != 0
		lBit := (ext & 0x40) != 0
		tBit := (ext & 0x20) != 0
		kBit := (ext & 0x10) != 0
		payloadIndex++

		if iBit {
			if payloadIndex >= len(payload) {
				return false
			}
			mBit := (payload[payloadIndex] & 0x80) != 0
			payloadIndex++
			if mBit {
				if payloadIndex >= len(payload) {
					return false
				}
				payloadIndex++
			}
		}
		if lBit {
			if payloadIndex >= len(payload) {
				return false
			}
			payloadIndex++
		}
		if tBit || kBit {
			if payloadIndex >= len(payload) {
				return false
			}
			payloadIndex++
		}
	}

	if payloadIndex < len(payload) {
		// Bit 0 of VP8 uncompressed frame header (P bit): 0 indicates Keyframe (I-frame), 1 indicates Inter-frame (P-frame)
		return (payload[payloadIndex] & 0x01) == 0
	}

	return false
}


// IsKeyframe parses the RTP payload and determines whether it contains a video Keyframe (I-frame / IDR)
// supporting both VP8 (RFC 7741), H.264 (RFC 6184), and VP9 codecs.
func IsKeyframe(mimeType string, payload []byte) bool {
	if len(payload) == 0 {
		return false
	}

	// 1. VP8 Keyframe Detection
	if mimeType == webrtc.MimeTypeVP8 || mimeType == "video/VP8" || mimeType == "" {
		if IsVP8Keyframe(payload) {
			return true
		}
	}

	// 2. H.264 Keyframe / IDR Slice Detection (RFC 6184)
	if mimeType == webrtc.MimeTypeH264 || mimeType == "video/H264" || mimeType == "" {
		nalType := payload[0] & 0x1F
		switch nalType {
		case 5, 7, 8: // IDR Slice (5), SPS (7), PPS (8)
			return true
		case 24: // STAP-A (Single-Time Aggregation Packet)
			offset := 1
			for offset+2 < len(payload) {
				naluSize := int(payload[offset])<<8 | int(payload[offset+1])
				offset += 2
				if offset < len(payload) {
					subNalType := payload[offset] & 0x1F
					if subNalType == 5 || subNalType == 7 || subNalType == 8 {
						return true
					}
				}
				offset += naluSize
			}
		case 28: // FU-A (Fragmentation Unit)
			if len(payload) > 1 {
				isStart := (payload[1] & 0x80) != 0
				fuNalType := payload[1] & 0x1F
				if isStart && (fuNalType == 5 || fuNalType == 7 || fuNalType == 8) {
					return true
				}
			}
		}
	}

	// 3. VP9 Keyframe Detection (IETF draft-ietf-payload-vp9)
	if mimeType == webrtc.MimeTypeVP9 || mimeType == "video/VP9" {
		if IsVP9Keyframe(payload) {
			return true
		}
	}

	return false
}
