package webrtc

import (
	"sync"
	"testing"
	"time"

	"github.com/pion/rtcp"
)

func TestBandwidthEstimator_InitializationAndGetters(t *testing.T) {
	be := NewBandwidthEstimator(2_000_000)

	if be.GetBitrate() != 2_000_000 {
		t.Fatalf("expected bitrate 2000000, got %d", be.GetBitrate())
	}
	if be.GetEstimatedBitrate() != 2_000_000 {
		t.Fatalf("expected estimated bitrate 2000000, got %d", be.GetEstimatedBitrate())
	}
	if be.GetPacketLoss() != 0.0 {
		t.Fatalf("expected packet loss 0.0, got %f", be.GetPacketLoss())
	}
	if be.GetRTT() != 50*time.Millisecond {
		t.Fatalf("expected RTT 50ms, got %v", be.GetRTT())
	}

	// Update
	be.Update(800_000, 0.05, 120*time.Millisecond)

	if be.GetBitrate() != 800_000 {
		t.Fatalf("expected bitrate 800000, got %d", be.GetBitrate())
	}
	if be.GetEstimatedBitrate() != 800_000 {
		t.Fatalf("expected estimated bitrate 800000, got %d", be.GetEstimatedBitrate())
	}
	if be.GetPacketLoss() != 0.05 {
		t.Fatalf("expected packet loss 0.05, got %f", be.GetPacketLoss())
	}
	if be.GetRTT() != 120*time.Millisecond {
		t.Fatalf("expected RTT 120ms, got %v", be.GetRTT())
	}
}

func TestBandwidthEstimator_ConcurrentAccess(t *testing.T) {
	be := NewBandwidthEstimator(1_000_000)
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				be.Update(1_000_000+id*1000+j, float64(j)*0.01, time.Duration(j)*time.Millisecond)
				_ = be.GetBitrate()
				_ = be.GetPacketLoss()
				_ = be.GetRTT()
			}
		}(i)
	}

	wg.Wait()
}

func TestBandwidthEstimator_ProcessTWCC(t *testing.T) {
	be := NewBandwidthEstimator(1_500_000)

	// Construct simulated TWCC packet with 10 packets (8 received, 2 not received)
	p := &rtcp.TransportLayerCC{
		BaseSequenceNumber: 100,
		PacketStatusCount:  10,
		PacketChunks: []rtcp.PacketStatusChunk{
			&rtcp.RunLengthChunk{
				PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta,
				RunLength:          8,
			},
			&rtcp.RunLengthChunk{
				PacketStatusSymbol: rtcp.TypeTCCPacketNotReceived,
				RunLength:          2,
			},
		},
	}

	loss := be.ProcessTWCC(p)
	// 2 lost out of 10 -> 20%
	if loss < 19.9 || loss > 20.1 {
		t.Fatalf("expected packet loss around 20%%, got %f%%", loss)
	}
	if be.GetPacketLoss() < 19.9 || be.GetPacketLoss() > 20.1 {
		t.Fatalf("expected GetPacketLoss around 20%%, got %f%%", be.GetPacketLoss())
	}
}

func TestBandwidthEstimator_ProcessReceiverReport(t *testing.T) {
	be := NewBandwidthEstimator(1_500_000)

	now := time.Now()
	compactNow := CompactNTP(now)

	// Simulate a 40ms RTT with 10ms DLSR
	// RTT = compactNow - LSR - DLSR = 40ms
	// So LSR + DLSR = compactNow - 40ms in 1/65536 units
	offset40ms := uint32(2621) // 40ms * 65536 / 1000 = 2621
	dlsr10ms := uint32(655)    // 10ms * 65536 / 1000 = 655
	lsr := compactNow - offset40ms - dlsr10ms

	rr := &rtcp.ReceiverReport{
		Reports: []rtcp.ReceptionReport{
			{
				SSRC:             12345,
				LastSenderReport: lsr,
				Delay:            dlsr10ms,
			},
		},
	}

	rtt := ExtractRTTFromReceiverReport(rr, now)
	if rtt < 35*time.Millisecond || rtt > 45*time.Millisecond {
		t.Fatalf("expected RTT around 40ms, got %v", rtt)
	}

	be.ProcessReceiverReport(rr)
	if be.GetRTT() < 35*time.Millisecond || be.GetRTT() > 45*time.Millisecond {
		t.Fatalf("expected estimator RTT around 40ms, got %v", be.GetRTT())
	}
}

func TestBandwidthEstimator_AIMD(t *testing.T) {
	be := NewBandwidthEstimator(1_000_000) // Initial 1,000,000 bps

	// 1. Packet loss < 2% -> increase by 5%
	newBitrate := be.ApplyAIMD(1.0) // 1% loss
	if newBitrate != 1_050_000 {
		t.Fatalf("expected bitrate 1050000 after 5%% increase, got %d", newBitrate)
	}

	// Another low loss iteration -> another 5% increase
	newBitrate = be.ApplyAIMD(0.5) // 0.5% loss
	expected := int(1_050_000 * 1.05)
	if newBitrate != expected {
		t.Fatalf("expected bitrate %d, got %d", expected, newBitrate)
	}

	// 2. Packet loss between 2% and 5% -> hold steady
	heldBitrate := be.ApplyAIMD(3.5)
	if heldBitrate != expected {
		t.Fatalf("expected bitrate to hold at %d, got %d", expected, heldBitrate)
	}

	// 3. Packet loss > 5% -> multiplicative decrease by 20%
	decreasedBitrate := be.ApplyAIMD(8.0) // 8% loss
	expectedDecreased := int(float64(expected) * 0.80)
	if decreasedBitrate != expectedDecreased {
		t.Fatalf("expected decreased bitrate %d, got %d", expectedDecreased, decreasedBitrate)
	}
}

func TestBandwidthEstimator_MonitorBandwidth(t *testing.T) {
	be := NewBandwidthEstimator(1_200_000)
	stopCh := make(chan struct{})

	called := make(chan int, 1)
	be.MonitorBandwidth(stopCh, func(bitrate int, loss float64, rtt time.Duration) {
		select {
		case called <- bitrate:
		default:
		}
	})

	close(stopCh) // Stop goroutine
}

func TestEvaluateBitrateLayer(t *testing.T) {
	// Bitrate > 1000kbps -> 'f'
	if layer := EvaluateBitrateLayer(1_500_000); layer != LayerHigh {
		t.Fatalf("expected layer 'f' for 1500kbps, got '%s'", layer)
	}
	if layer := EvaluateBitrateLayer(1_001_000); layer != LayerHigh {
		t.Fatalf("expected layer 'f' for 1001kbps, got '%s'", layer)
	}

	// Bitrate > 500kbps and <= 1000kbps -> 'h'
	if layer := EvaluateBitrateLayer(800_000); layer != LayerMedium {
		t.Fatalf("expected layer 'h' for 800kbps, got '%s'", layer)
	}
	if layer := EvaluateBitrateLayer(501_000); layer != LayerMedium {
		t.Fatalf("expected layer 'h' for 501kbps, got '%s'", layer)
	}
	if layer := EvaluateBitrateLayer(1_000_000); layer != LayerMedium {
		t.Fatalf("expected layer 'h' for 1000kbps, got '%s'", layer)
	}

	// Bitrate <= 500kbps -> 'q'
	if layer := EvaluateBitrateLayer(500_000); layer != LayerLow {
		t.Fatalf("expected layer 'q' for 500kbps, got '%s'", layer)
	}
	if layer := EvaluateBitrateLayer(200_000); layer != LayerLow {
		t.Fatalf("expected layer 'q' for 200kbps, got '%s'", layer)
	}
}
