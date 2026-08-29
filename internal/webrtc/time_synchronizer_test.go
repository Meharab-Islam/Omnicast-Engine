package webrtc

import (
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

func TestTimeSynchronizer_LipSyncRewriting(t *testing.T) {
	tsAdjuster := NewTimestampAdjuster()
	// Simulate video layer switch with an offset of +90000 (1 second at 90kHz)
	_ = tsAdjuster.AdjustContinuous(1000, 3000)
	tsAdjuster.Switch(5000, 3000) // Layer switch occurs

	timeSync := NewTimeSynchronizer(tsAdjuster)

	now := time.Now()
	ntpTime := TimeToNtp(now)

	// 1. Process Publisher's video Sender Report
	publisherVideoSR := &rtcp.SenderReport{
		SSRC:        123456,
		NTPTime:     ntpTime,
		RTPTime:     100000,
		PacketCount: 50,
		OctetCount:  50000,
	}
	timeSync.ProcessSenderReport(publisherVideoSR, webrtc.RTPCodecTypeVideo)

	// 2. Process Publisher's audio Sender Report
	publisherAudioSR := &rtcp.SenderReport{
		SSRC:        654321,
		NTPTime:     ntpTime,
		RTPTime:     48000,
		PacketCount: 100,
		OctetCount:  10000,
	}
	timeSync.ProcessSenderReport(publisherAudioSR, webrtc.RTPCodecTypeAudio)

	// 3. Generate Rewritten Sender Report for Viewer Video track
	viewerVideoSR := timeSync.GenerateRewrittenSenderReport(webrtc.RTPCodecTypeVideo, 999999, 10, 5000)
	if viewerVideoSR == nil {
		t.Fatalf("Expected non-nil viewer video SenderReport")
	}
	if viewerVideoSR.SSRC != 999999 {
		t.Errorf("Expected outgoing SSRC 999999, got %d", viewerVideoSR.SSRC)
	}
	if viewerVideoSR.RTPTime == 0 {
		t.Errorf("Expected non-zero RTP timestamp")
	}

	// 4. Generate Rewritten Sender Report for Viewer Audio track
	viewerAudioSR := timeSync.GenerateRewrittenSenderReport(webrtc.RTPCodecTypeAudio, 888888, 20, 2000)
	if viewerAudioSR == nil {
		t.Fatalf("Expected non-nil viewer audio SenderReport")
	}
	if viewerAudioSR.SSRC != 888888 {
		t.Errorf("Expected outgoing SSRC 888888, got %d", viewerAudioSR.SSRC)
	}

	// 5. Test NTP Time conversion accuracy
	convTime := NtpToTime(ntpTime)
	diff := now.Sub(convTime)
	if diff < -time.Millisecond || diff > time.Millisecond {
		t.Errorf("NTP conversion drift too large: %v", diff)
	}
}
