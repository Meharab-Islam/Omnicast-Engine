package webrtc

import (
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

// TrackSwitcher intercepts incoming simulcast RTP packets and smoothly relays them to a viewer
// by rewriting RTP Sequence Numbers and Timestamps to ensure uninterrupted video decoding during layer switches.
// Holds references to the 3 incoming Host tracks (RID 'q', 'h', 'f') and one outgoing webrtc.TrackLocalStaticRTP for the Viewer.
type TrackSwitcher struct {
	// 3 incoming Host tracks for Simulcast layers
	trackQ *webrtc.TrackRemote // Low Resolution (RID 'q')
	trackH *webrtc.TrackRemote // Medium Resolution (RID 'h')
	trackF *webrtc.TrackRemote // High / Full Resolution (RID 'f')

	// Active Host track pointer currently being forwarded to the viewer
	activeTrack *webrtc.TrackRemote

	// Target Host track pointer to switch to
	targetTrack *webrtc.TrackRemote

	// Flag indicating whether a track switch is pending
	pendingSwitch bool

	// One outgoing track for the Viewer
	outTrack *webrtc.TrackLocalStaticRTP

	currentLayer    string
	targetLayer     string
	waitingKeyframe bool

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

	started   bool
	queue     chan *rtp.Packet
	closed    chan struct{}
	closeOnce sync.Once
	mu        sync.RWMutex
}

// NewTrackSwitcher creates and initializes a new TrackSwitcher with a default layer (e.g., 'h' or 'f')
func NewTrackSwitcher(outTrack *webrtc.TrackLocalStaticRTP, initialLayer string) *TrackSwitcher {
	if initialLayer == "" {
		initialLayer = "h"
	}
	ts := &TrackSwitcher{
		outTrack:     outTrack,
		currentLayer: initialLayer,
		targetLayer:  initialLayer,
		seqAdjuster:  NewSequenceNumberAdjuster(),
		tsAdjuster:   NewTimestampAdjuster(),
		queue:        make(chan *rtp.Packet, 256),
		closed:       make(chan struct{}),
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
				}
			}
		}
	}()

	return ts
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

// SwitchLayer requests a switch to a target simulcast layer RID.
// It compares the resolved target track with the activeTrack:
// If they are different, sets pendingSwitch = true and stores targetTrack.
func (ts *TrackSwitcher) SwitchLayer(targetRID string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	var targetTrack *webrtc.TrackRemote
	switch targetRID {
	case LayerLow:
		targetTrack = ts.trackQ
	case LayerMedium:
		targetTrack = ts.trackH
	case LayerHigh:
		targetTrack = ts.trackF
	}

	ts.targetLayer = targetRID

	// Compare target track with activeTrack
	if targetTrack != ts.activeTrack || ts.currentLayer != targetRID {
		ts.pendingSwitch = true
		ts.targetTrack = targetTrack
		ts.waitingKeyframe = true
	} else {
		ts.pendingSwitch = false
		ts.targetTrack = nil
		ts.waitingKeyframe = false
	}
}

// ForceSwitchLayer switches the layer immediately
func (ts *TrackSwitcher) ForceSwitchLayer(targetRID string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.currentLayer = targetRID
	ts.targetLayer = targetRID
	ts.pendingSwitch = false
	ts.waitingKeyframe = false
	switch targetRID {
	case LayerLow:
		ts.activeTrack = ts.trackQ
	case LayerMedium:
		ts.activeTrack = ts.trackH
	case LayerHigh:
		ts.activeTrack = ts.trackF
	}
}

// WriteRTP processes an incoming RTP packet from a specific RID, rewrites its SequenceNumber & Timestamp,
// and pushes it to the non-blocking worker queue for asynchronous transmission.
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

	// In the RTP forwarding loop, before writing the packet, check if pendingSwitch == true
	if ts.pendingSwitch || ts.waitingKeyframe {
		// If this packet is from the target layer/track, parse the incoming RTP payload to check if it's a Keyframe (I-frame)
		if rid == ts.targetLayer || (ts.targetTrack != nil && rid == ts.targetTrack.RID()) {
			codecMime := ""
			if ts.outTrack != nil {
				codecMime = ts.outTrack.Codec().MimeType
			}
			isKey := IsKeyframe(codecMime, packet.Payload)

			// Once a keyframe is detected on the targetTrack, change the activeTrack pointer to the targetTrack
			if isKey || !ts.started {
				ts.currentLayer = ts.targetLayer
				// Ensure targetTrack is resolved if switching by RID
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

				// Immediately upon switching, calculate the difference between the old track's
				// sequence number and the new track's sequence number to update the SequenceNumberAdjuster offset.
				// Calculate the timestamp offset similarly and update the TimestampAdjuster.
				if ts.started {
					// Use the dedicated SequenceNumberAdjuster to compute: offset = (lastOutSeq + 1) - newInSeq
					ts.seqAdjuster.Switch(packet.SequenceNumber)
					ts.seqOffset = ts.seqAdjuster.GetOffset()

					// Use the dedicated TimestampAdjuster to compute: offset = (lastOutTS + frameDuration) - newInTS
					ts.tsAdjuster.Switch(packet.Timestamp, DefaultFrameDuration90kHz)
					ts.tsOffset = ts.tsAdjuster.GetOffset()
				}
			} else {
				// If the packet is NOT a keyframe, drop packets from the targetTrack
				return nil
			}
		}
	}

	// Continue forwarding packets from the old activeTrack and drop packets that do not belong to the currently active layer
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
		// Initialize the SequenceNumberAdjuster and TimestampAdjuster with the first packet
		ts.seqAdjuster.Adjust(packet.SequenceNumber)
		ts.tsAdjuster.Adjust(packet.Timestamp)
	} else {
		// Apply the adjusted sequence numbers and timestamps to the RTP packet headers
		// before calling WriteRTP() to ensure a completely seamless transition for the Viewer.
		//
		// SequenceNumberAdjuster: outSeq = inSeq + offset  (offset recalculated on each layer switch)
		// TimestampAdjuster:      outTS  = inTS  + offset  (offset recalculated on each layer switch)
		//
		// This guarantees the Viewer's decoder sees a strictly monotonic sequence space
		// and a continuous timestamp clock, regardless of which simulcast layer is active.
		outPkt.Header.SequenceNumber = ts.seqAdjuster.Adjust(packet.SequenceNumber)
		outPkt.Header.Timestamp = ts.tsAdjuster.Adjust(packet.Timestamp)

		ts.lastInSeq = packet.SequenceNumber
		ts.lastInTS = packet.Timestamp
		ts.lastOutSeq = outPkt.Header.SequenceNumber
		ts.lastOutTS = outPkt.Header.Timestamp
	}

	// Enqueue the rewritten packet for the background worker to call outTrack.WriteRTP()
	// Non-blocking write: if the viewer's buffer is full, drop packet for this slow viewer without blocking others
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
		iBit := (payload[1] & 0x80) != 0
		lBit := (payload[1] & 0x40) != 0
		tBit := (payload[1] & 0x20) != 0
		kBit := (payload[1] & 0x10) != 0
		payloadIndex++

		if iBit {
			if payloadIndex >= len(payload) {
				return false
			}
			mBit := (payload[payloadIndex] & 0x80) != 0
			payloadIndex++
			if mBit {
				payloadIndex++
			}
		}
		if lBit {
			payloadIndex++
		}
		if tBit || kBit {
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
// supporting both VP8 (RFC 7741) and H.264 (RFC 6184) codecs.
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
		case 28: // FU-A (Fragmentation Unit)
			if len(payload) > 1 {
				isStart := (payload[1] & 0x80) != 0
				fuNalType := payload[1] & 0x1F
				if isStart && (fuNalType == 5 || fuNalType == 7) {
					return true
				}
			}
		}
	}

	return false
}
