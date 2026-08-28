package webrtc

import (
	"log"
	"sync"
	"time"

	"github.com/pion/rtcp"
)

type packetLossEntry struct {
	timestamp time.Time
	total     int
	lost      int
}

// BandwidthEstimator tracks and estimates network capacity, packet loss, and round-trip time (RTT)
// for an active subscriber over sliding windows to drive dynamic Simulcast layer switching and adaptive bitrate (ABR).
type BandwidthEstimator struct {
	mu             sync.RWMutex
	currentBitrate int               // Current estimated bandwidth in bps
	packetLoss     float64           // Packet loss percentage over 1-second window (0.0% - 100.0%)
	rtt            time.Duration     // Round-Trip Time latency
	lossHistory    []packetLossEntry // Sliding 1-second window loss history
}

// NewBandwidthEstimator creates and initializes a new BandwidthEstimator with default metrics
func NewBandwidthEstimator(initialBitrate int) *BandwidthEstimator {
	if initialBitrate <= 0 {
		initialBitrate = 1_500_000 // 1.5 Mbps default
	}
	return &BandwidthEstimator{
		currentBitrate: initialBitrate,
		packetLoss:     0.0,
		rtt:            50 * time.Millisecond,
		lossHistory:    make([]packetLossEntry, 0),
	}
}

// GetBitrate returns the current estimated bitrate in bps
func (be *BandwidthEstimator) GetBitrate() int {
	be.mu.RLock()
	defer be.mu.RUnlock()
	return be.currentBitrate
}

// GetEstimatedBitrate returns the Viewer's real-time estimated network capacity in bps in a thread-safe manner
func (be *BandwidthEstimator) GetEstimatedBitrate() int {
	be.mu.RLock()
	defer be.mu.RUnlock()
	return be.currentBitrate
}

// GetPacketLoss returns the current packet loss percentage over the last 1-second window
func (be *BandwidthEstimator) GetPacketLoss() float64 {
	be.mu.RLock()
	defer be.mu.RUnlock()
	return be.packetLoss
}

// GetRTT returns the current Round-Trip Time
func (be *BandwidthEstimator) GetRTT() time.Duration {
	be.mu.RLock()
	defer be.mu.RUnlock()
	return be.rtt
}

// AIMD constants for adaptive bitrate rate-control
const (
	MinBitrateBps     = 100_000   // 100 kbps minimum floor
	MaxBitrateBps     = 5_000_000 // 5 Mbps maximum ceiling
	AIMDIncreaseRatio = 0.05      // 5% increase when loss < 2%
	AIMDDecreaseRatio = 0.20      // 20% decrease when loss > 5%
	LossThresholdLow  = 2.0       // 2% packet loss threshold
	LossThresholdHigh = 5.0       // 5% packet loss threshold
)

// ApplyAIMD executes the Additive Increase, Multiplicative Decrease algorithm:
// - If packet loss is < 2%, increase currentBitrate by 5% (up to MaxBitrateBps).
// - If packet loss is > 5%, decrease currentBitrate by 20% (down to MinBitrateBps) to prevent network congestion.
// - If packet loss is between 2% and 5%, maintains the current bitrate.
func (be *BandwidthEstimator) ApplyAIMD(packetLoss float64) int {
	be.mu.Lock()
	defer be.mu.Unlock()

	be.packetLoss = packetLoss

	// 1. If packet loss is < 2%, increase currentBitrate by 5%
	if packetLoss < LossThresholdLow {
		increasedBitrate := int(float64(be.currentBitrate) * (1.0 + AIMDIncreaseRatio))
		if increasedBitrate > MaxBitrateBps {
			increasedBitrate = MaxBitrateBps
		}
		be.currentBitrate = increasedBitrate
	} else if packetLoss > LossThresholdHigh {
		// 2. If packet loss is > 5%, decrease currentBitrate by 20% to prevent network congestion
		decreasedBitrate := int(float64(be.currentBitrate) * (1.0 - AIMDDecreaseRatio))
		if decreasedBitrate < MinBitrateBps {
			decreasedBitrate = MinBitrateBps
		}
		be.currentBitrate = decreasedBitrate
	}

	return be.currentBitrate
}

// Update updates the estimator metrics in a thread-safe manner
func (be *BandwidthEstimator) Update(bitrate int, loss float64, rtt time.Duration) {
	be.mu.Lock()
	defer be.mu.Unlock()
	if bitrate > 0 {
		be.currentBitrate = bitrate
	}
	if loss >= 0 {
		be.packetLoss = loss
	}
	if rtt >= 0 {
		be.rtt = rtt
	}
}

// ParseTWCCLoss extracts the count of total expected packets and lost packets from an rtcp.TransportLayerCC packet
func ParseTWCCLoss(p *rtcp.TransportLayerCC) (totalPackets int, lostPackets int) {
	if p == nil {
		return 0, 0
	}

	for _, chunk := range p.PacketChunks {
		switch c := chunk.(type) {
		case *rtcp.RunLengthChunk:
			count := int(c.RunLength)
			totalPackets += count
			if c.PacketStatusSymbol == rtcp.TypeTCCPacketNotReceived {
				lostPackets += count
			}
		case *rtcp.StatusVectorChunk:
			for _, symbol := range c.SymbolList {
				totalPackets++
				if symbol == rtcp.TypeTCCPacketNotReceived {
					lostPackets++
				}
			}
		}
	}

	// Fallback calculation using PacketStatusCount vs received deltas
	if totalPackets == 0 && p.PacketStatusCount > 0 {
		totalPackets = int(p.PacketStatusCount)
		received := len(p.RecvDeltas)
		if received < totalPackets {
			lostPackets = totalPackets - received
		}
	}

	return totalPackets, lostPackets
}

// ProcessTWCC processes an incoming TWCC feedback report, parses the received and lost packet counts,
// calculates the precise packet loss percentage over the last 1-second sliding window, and updates the estimator.
func (be *BandwidthEstimator) ProcessTWCC(p *rtcp.TransportLayerCC) float64 {
	if p == nil {
		return be.GetPacketLoss()
	}

	total, lost := ParseTWCCLoss(p)
	if total == 0 {
		return be.GetPacketLoss()
	}

	be.mu.Lock()
	defer be.mu.Unlock()

	now := time.Now()
	// Record new sample into history
	be.lossHistory = append(be.lossHistory, packetLossEntry{
		timestamp: now,
		total:     total,
		lost:      lost,
	})

	// Prune records older than 1 second (1000ms window)
	windowStart := now.Add(-1 * time.Second)
	firstValid := 0
	for i, entry := range be.lossHistory {
		if !entry.timestamp.Before(windowStart) {
			firstValid = i
			break
		}
	}
	if firstValid > 0 {
		be.lossHistory = be.lossHistory[firstValid:]
	}

	// Aggregate total packets and lost packets in the active 1-second window
	windowTotal := 0
	windowLost := 0
	for _, entry := range be.lossHistory {
		if !entry.timestamp.Before(windowStart) {
			windowTotal += entry.total
			windowLost += entry.lost
		}
	}

	var lossPercent float64
	if windowTotal > 0 {
		lossPercent = (float64(windowLost) / float64(windowTotal)) * 100.0
	}
	be.packetLoss = lossPercent
	return lossPercent
}

// CompactNTP computes the middle 32 bits of the current time in NTP format (RFC 3550)
func CompactNTP(t time.Time) uint32 {
	// Seconds between Jan 1 1900 and Jan 1 1970 (NTP epoch offset)
	const ntpEpochOffset = 2208988800
	ntpSec := uint32(t.Unix() + ntpEpochOffset)
	ntpFrac := uint32((uint64(t.Nanosecond()) << 32) / 1_000_000_000)
	return (ntpSec << 16) | (ntpFrac >> 16)
}

// ExtractRTTFromReceiverReport parses an rtcp.ReceiverReport (RFC 3550 Section 6.4.1)
// using the Last Sender Report (LSR) and Delay Since Last Sender Report (DLSR) to calculate the measured RTT.
func ExtractRTTFromReceiverReport(rr *rtcp.ReceiverReport, arrivalTime time.Time) time.Duration {
	if rr == nil || len(rr.Reports) == 0 {
		return 0
	}

	if arrivalTime.IsZero() {
		arrivalTime = time.Now()
	}
	compactNow := CompactNTP(arrivalTime)

	var bestRTT time.Duration
	for _, report := range rr.Reports {
		if report.LastSenderReport == 0 {
			continue
		}
		// RTT = A - LSR - DLSR (expressed in units of 1/65536 seconds)
		diff := compactNow - report.LastSenderReport - report.Delay
		rttSeconds := float64(diff) / 65536.0
		if rttSeconds > 0 && rttSeconds < 10.0 { // Filter negative or unrealistic values
			rtt := time.Duration(rttSeconds * float64(time.Second))
			if bestRTT == 0 || rtt < bestRTT {
				bestRTT = rtt
			}
		}
	}

	return bestRTT
}

// ProcessReceiverReport parses the RTCP Receiver Report (RR), calculates RTT and fraction loss,
// and updates the BandwidthEstimator.
func (be *BandwidthEstimator) ProcessReceiverReport(rr *rtcp.ReceiverReport) time.Duration {
	if rr == nil {
		return be.GetRTT()
	}

	rtt := ExtractRTTFromReceiverReport(rr, time.Now())
	if rtt > 0 {
		be.mu.Lock()
		be.rtt = rtt
		be.mu.Unlock()
	}

	return be.GetRTT()
}

// EvaluateBitrateLayer determines the target simulcast layer based on estimated bitrate:
// - Target 'f' if bitrate > 1000 kbps (1,000,000 bps)
// - Target 'h' if bitrate > 500 kbps (500,000 bps)
// - Else 'q'
func EvaluateBitrateLayer(bitrateBps int) string {
	bitrateKbps := bitrateBps / 1000
	if bitrateKbps > 1000 {
		return LayerHigh // 'f'
	} else if bitrateKbps > 500 {
		return LayerMedium // 'h'
	}
	return LayerLow // 'q'
}

// MonitorBandwidth launches a background goroutine for a Viewer that calls GetEstimatedBitrate()
// every 2 seconds, calculates target layer ('f' if >1000kbps, 'h' if >500kbps, else 'q'),
// logs the real-time capacity and quality metrics, and triggers optional callbacks.
func (be *BandwidthEstimator) MonitorBandwidth(stopCh <-chan struct{}, onTick func(bitrate int, loss float64, rtt time.Duration)) {
	if be == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				bitrate := be.GetEstimatedBitrate()
				loss := be.GetPacketLoss()
				rtt := be.GetRTT()

				// Thresholds: Target 'f' if bitrate > 1000kbps, 'h' if > 500kbps, else 'q'
				var targetLayer string
				bitrateKbps := bitrate / 1000
				if bitrateKbps > 1000 {
					targetLayer = LayerHigh // 'f'
				} else if bitrateKbps > 500 {
					targetLayer = LayerMedium // 'h'
				} else {
					targetLayer = LayerLow // 'q'
				}

				log.Printf("[Bandwidth Monitor] Estimated Bitrate: %d bps (%d kbps) -> Target Layer: '%s' | Loss: %.2f%% | RTT: %v\n",
					bitrate, bitrateKbps, targetLayer, loss, rtt)

				if onTick != nil {
					onTick(bitrate, loss, rtt)
				}
			}
		}
	}()
}

// MonitorBandwidth runs a background goroutine for a Viewer that calls GetEstimatedBitrate() every 2 seconds.
func MonitorBandwidth(be *BandwidthEstimator, stopCh <-chan struct{}, onTick func(bitrate int, loss float64, rtt time.Duration)) {
	if be == nil {
		return
	}
	be.MonitorBandwidth(stopCh, onTick)
}
