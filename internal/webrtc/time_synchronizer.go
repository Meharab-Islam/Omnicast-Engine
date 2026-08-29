package webrtc

import (
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

// TimeSynchronizer tracks the mapping between the Publisher's NTP (Network Time Protocol) wall-clock time
// and adjusted outgoing RTP timestamps to guarantee precise Audio/Video Lip-Sync (RFC 3550 & WebRTC specs).
type TimeSynchronizer struct {
	mu sync.RWMutex

	// Latest incoming Publisher Sender Reports
	latestVideoNTP uint64
	latestVideoRTP uint32
	videoSSRC      uint32
	videoClockRate uint32 // typically 90000 for Video (H.264/VP8/VP9)

	latestAudioNTP uint64
	latestAudioRTP uint32
	audioSSRC      uint32
	audioClockRate uint32 // typically 48000 for Audio (Opus)

	// Dedicated TimestampAdjuster for outgoing video stream (e.g. from TrackSwitcher)
	videoTSAdjuster *TimestampAdjuster

	// Reference time
	lastUpdated time.Time
}

// NewTimeSynchronizer creates a new TimeSynchronizer instance
func NewTimeSynchronizer(videoTSAdjuster *TimestampAdjuster) *TimeSynchronizer {
	return &TimeSynchronizer{
		videoClockRate:  90000,
		audioClockRate:  48000,
		videoTSAdjuster: videoTSAdjuster,
		lastUpdated:     time.Now(),
	}
}

// SetTimestampAdjuster updates the video timestamp adjuster
func (ts *TimeSynchronizer) SetTimestampAdjuster(adj *TimestampAdjuster) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.videoTSAdjuster = adj
}

// ProcessSenderReport intercepts an incoming rtcp.SenderReport from the Publisher
func (ts *TimeSynchronizer) ProcessSenderReport(sr *rtcp.SenderReport, kind webrtc.RTPCodecType) {
	if sr == nil {
		return
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if kind == webrtc.RTPCodecTypeVideo {
		ts.latestVideoNTP = sr.NTPTime
		ts.latestVideoRTP = sr.RTPTime
		ts.videoSSRC = sr.SSRC
	} else if kind == webrtc.RTPCodecTypeAudio {
		ts.latestAudioNTP = sr.NTPTime
		ts.latestAudioRTP = sr.RTPTime
		ts.audioSSRC = sr.SSRC
	}
	ts.lastUpdated = time.Now()
}

// GenerateRewrittenSenderReport calculates and injects newly adjusted RTP timestamps matching
// the outgoing stream and current NTP wall-clock time for the Viewer
func (ts *TimeSynchronizer) GenerateRewrittenSenderReport(kind webrtc.RTPCodecType, outSSRC uint32, packetCount uint32, octetCount uint32) *rtcp.SenderReport {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	now := time.Now()
	ntpTime := TimeToNtp(now)

	var rtpTime uint32

	if kind == webrtc.RTPCodecTypeVideo {
		if ts.latestVideoNTP != 0 {
			// Elapsed duration since incoming Publisher SR
			elapsed := now.Sub(NtpToTime(ts.latestVideoNTP))
			rtpDelta := uint32(elapsed.Seconds() * float64(ts.videoClockRate))
			baseRTP := ts.latestVideoRTP + rtpDelta
			if ts.videoTSAdjuster != nil {
				rtpTime = ts.videoTSAdjuster.Rewrite(baseRTP)
			} else {
				rtpTime = baseRTP
			}
		} else {
			// Fallback base monotonic timestamp
			rtpTime = uint32(now.UnixNano() / int64(time.Second/time.Duration(ts.videoClockRate)))
			if ts.videoTSAdjuster != nil {
				rtpTime = ts.videoTSAdjuster.Rewrite(rtpTime)
			}
		}
	} else {
		// Audio track
		if ts.latestAudioNTP != 0 {
			elapsed := now.Sub(NtpToTime(ts.latestAudioNTP))
			rtpDelta := uint32(elapsed.Seconds() * float64(ts.audioClockRate))
			rtpTime = ts.latestAudioRTP + rtpDelta
		} else {
			rtpTime = uint32(now.UnixNano() / int64(time.Second/time.Duration(ts.audioClockRate)))
		}
	}

	return &rtcp.SenderReport{
		SSRC:        outSSRC,
		NTPTime:     ntpTime,
		RTPTime:     rtpTime,
		PacketCount: packetCount,
		OctetCount:  octetCount,
	}
}

// TimeToNtp converts a Go time.Time to a standard 64-bit NTP timestamp
func TimeToNtp(t time.Time) uint64 {
	// Offset between NTP epoch (1900-01-01) and Unix epoch (1970-01-01) in seconds
	const ntpEpochOffset = 2208988800
	sec := uint64(t.Unix() + ntpEpochOffset)
	nsec := uint64(t.Nanosecond())
	// Convert nanoseconds to 2^32 fractional units
	frac := (nsec << 32) / 1000000000
	return (sec << 32) | frac
}

// NtpToTime converts a 64-bit NTP timestamp to Go time.Time
func NtpToTime(ntp uint64) time.Time {
	const ntpEpochOffset = 2208988800
	sec := int64((ntp >> 32) - ntpEpochOffset)
	frac := ntp & 0xFFFFFFFF
	nsec := (frac * 1000000000) >> 32
	return time.Unix(sec, int64(nsec)).UTC()
}

// StartPeriodicSenderReports starts a background goroutine sending rewritten rtcp.SenderReport packets
// to the viewer every 1 second to maintain perfect A/V Lip-Sync alignment.
func StartPeriodicSenderReports(pc *webrtc.PeerConnection, videoSender *webrtc.RTPSender, audioSender *webrtc.RTPSender, timeSync *TimeSynchronizer, stopCh <-chan struct{}) {
	if pc == nil || timeSync == nil {
		return
	}

	ticker := time.NewTicker(1 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
					return
				}
				var reports []rtcp.Packet

				if videoSender != nil && videoSender.Track() != nil {
					ssrc := getSenderSSRC(videoSender)
					sr := timeSync.GenerateRewrittenSenderReport(webrtc.RTPCodecTypeVideo, ssrc, 0, 0)
					reports = append(reports, sr)
				}
				if audioSender != nil && audioSender.Track() != nil {
					ssrc := getSenderSSRC(audioSender)
					sr := timeSync.GenerateRewrittenSenderReport(webrtc.RTPCodecTypeAudio, ssrc, 0, 0)
					reports = append(reports, sr)
				}

				if len(reports) > 0 {
					_ = pc.WriteRTCP(reports)
				}
			case <-stopCh:
				return
			}
		}
	}()
}

func getSenderSSRC(sender *webrtc.RTPSender) uint32 {
	if sender == nil {
		return 0
	}
	params := sender.GetParameters()
	if len(params.Encodings) > 0 && params.Encodings[0].SSRC != 0 {
		return uint32(params.Encodings[0].SSRC)
	}
	return 0
}
