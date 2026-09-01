package webrtc

import (
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

// PacerQueueItem represents an RTP packet queued for paced transmission
type PacerQueueItem struct {
	Track  *webrtc.TrackLocalStaticRTP
	Packet *rtp.Packet
	Size   int
}

// LeakyBucketPacer implements an enterprise Leaky Bucket rate limiter for RTP egress packets
// to eliminate micro-bursts, prevent network bufferbloat, and smooth out packet delivery.
type LeakyBucketPacer struct {
	queue        chan PacerQueueItem
	bitrateBps   int64
	maxBurstSize int
	mu           sync.RWMutex
	stopChan     chan struct{}
}

// NewLeakyBucketPacer creates a new paced transmission queue with a target bitrate (e.g., 5 Mbps default)
func NewLeakyBucketPacer(bitrateBps int64, queueCapacity int) *LeakyBucketPacer {
	if bitrateBps <= 0 {
		bitrateBps = 5000000 // 5 Mbps default pacing rate
	}
	if queueCapacity <= 0 {
		queueCapacity = 4096 // Enterprise queue depth
	}

	pacer := &LeakyBucketPacer{
		queue:        make(chan PacerQueueItem, queueCapacity),
		bitrateBps:   bitrateBps,
		maxBurstSize: 1500 * 10,
		stopChan:     make(chan struct{}),
	}

	go pacer.startPacingLoop()
	return pacer
}

// SetBitrate dynamically updates the target pacing bitrate based on TWCC/ABR feedback
func (p *LeakyBucketPacer) SetBitrate(bitrateBps int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if bitrateBps > 0 {
		p.bitrateBps = bitrateBps
	}
}

// Enqueue adds an outgoing RTP packet to the pacer queue.
// If the queue is saturated, it delivers immediately to prevent memory starvation.
func (p *LeakyBucketPacer) Enqueue(track *webrtc.TrackLocalStaticRTP, packet *rtp.Packet) {
	if track == nil || packet == nil {
		return
	}

	item := PacerQueueItem{
		Track:  track,
		Packet: packet,
		Size:   len(packet.Payload) + 12,
	}

	select {
	case p.queue <- item:
	default:
		// Queue full (extreme congestion) -> write directly to avoid packet buildup
		_ = track.WriteRTP(packet)
	}
}

func (p *LeakyBucketPacer) startPacingLoop() {
	for {
		select {
		case <-p.stopChan:
			return
		case item := <-p.queue:
			if item.Track != nil && item.Packet != nil {
				_ = item.Track.WriteRTP(item.Packet)

				// Calculate pacing delay for packet size at current bitrate
				p.mu.RLock()
				rate := p.bitrateBps
				p.mu.RUnlock()
				if rate > 0 {
					delayNs := int64(item.Size) * 8 * 1000000000 / rate
					if delayNs > 0 && delayNs < 5000000 { // Cap at 5ms per packet
						time.Sleep(time.Duration(delayNs) * time.Nanosecond)
					}
				}
			}
		}
	}
}

// Stop terminates the pacer goroutine
func (p *LeakyBucketPacer) Stop() {
	select {
	case <-p.stopChan:
	default:
		close(p.stopChan)
	}
}
