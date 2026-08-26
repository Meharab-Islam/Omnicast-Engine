package webrtc

import (
	"os"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
)

// InitWebRTC initializes Pion WebRTC MediaEngine with VP8, H264 video codecs,
// and Opus audio codec, registers RTP Header Extensions (MID, RID, Repair RID, TWCC, ABS-Send-Time)
// for Simulcast and Bandwidth Estimation, registers default interceptors (NACK, RTCP Reports, TWCC/REMB),
// and returns a globally usable *webrtc.API instance.
func InitWebRTC() (*webrtc.API, error) {
	// Create a MediaEngine object to configure supported codecs
	mediaEngine := &webrtc.MediaEngine{}

	// Register Opus Audio Codec
	if err := mediaEngine.RegisterCodec(
		webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     webrtc.MimeTypeOpus,
				ClockRate:    48000,
				Channels:     2,
				SDPFmtpLine:  "minptime=10;useinbandfec=1",
				RTCPFeedback: nil,
			},
			PayloadType: 111,
		},
		webrtc.RTPCodecTypeAudio,
	); err != nil {
		return nil, err
	}

	// Register VP8 Video Codec
	if err := mediaEngine.RegisterCodec(
		webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP8,
				ClockRate: 90000,
				Channels:  0,
				RTCPFeedback: []webrtc.RTCPFeedback{
					{Type: "goog-remb", Parameter: ""},
					{Type: "ccm", Parameter: "fir"},
					{Type: "nack", Parameter: ""},
					{Type: "nack", Parameter: "pli"},
				},
			},
			PayloadType: 96,
		},
		webrtc.RTPCodecTypeVideo,
	); err != nil {
		return nil, err
	}

	// Register H264 Video Codec
	if err := mediaEngine.RegisterCodec(
		webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeH264,
				ClockRate:   90000,
				Channels:    0,
				SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
				RTCPFeedback: []webrtc.RTCPFeedback{
					{Type: "goog-remb", Parameter: ""},
					{Type: "ccm", Parameter: "fir"},
					{Type: "nack", Parameter: ""},
					{Type: "nack", Parameter: "pli"},
				},
			},
			PayloadType: 97,
		},
		webrtc.RTPCodecTypeVideo,
	); err != nil {
		return nil, err
	}

	// 1. Register RTP Header Extensions for Simulcast (MID & RID) and Bandwidth Estimation
	// MID (Media Stream ID) & RID (RTP Stream ID: 'q', 'h', 'f') allow the SFU server
	// to identify which incoming RTP packets belong to which Simulcast layer (Low, Medium, High).
	videoHeaderExtensions := []string{
		sdp.SDESMidURI,
		sdp.SDESRTPStreamIDURI,
		"urn:ietf:params:rtp-hdrext:sdes:repaired-rtp-stream-id",
		sdp.TransportCCURI,
		sdp.ABSSendTimeURI,
	}
	for _, uri := range videoHeaderExtensions {
		if err := mediaEngine.RegisterHeaderExtension(
			webrtc.RTPHeaderExtensionCapability{URI: uri},
			webrtc.RTPCodecTypeVideo,
		); err != nil {
			return nil, err
		}
	}

	audioHeaderExtensions := []string{
		sdp.SDESMidURI,
		sdp.AudioLevelURI,
	}
	for _, uri := range audioHeaderExtensions {
		if err := mediaEngine.RegisterHeaderExtension(
			webrtc.RTPHeaderExtensionCapability{URI: uri},
			webrtc.RTPCodecTypeAudio,
		); err != nil {
			return nil, err
		}
	}

	// 2. Register Interceptors (NACK, RTCP reports, TWCC/REMB Congestion Control / Bandwidth Estimation)
	interceptorRegistry := &interceptor.Registry{}

	// Register TWCC Header Extension Sender so remote peers can generate TWCC feedback
	if err := webrtc.ConfigureTWCCHeaderExtensionSender(mediaEngine, interceptorRegistry); err != nil {
		return nil, err
	}

	// Register Default Interceptors (NACK Generator & Responder, RTCP Reports, TWCC Responder)
	if err := webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry); err != nil {
		return nil, err
	}

	// 3. Configure SettingEngine to restrict WebRTC UDP port range (50000 - 50050) and advertise NAT 1:1 Public IP
	settingEngine := webrtc.SettingEngine{}
	if err := settingEngine.SetEphemeralUDPPortRange(50000, 50050); err != nil {
		return nil, err
	}

	publicIP := os.Getenv("PUBLIC_IP")
	if publicIP == "" {
		publicIP = "192.168.0.116" // Fallback local IP
	}
	settingEngine.SetNAT1To1IPs([]string{publicIP}, webrtc.ICECandidateTypeHost)

	// Configure ICE timeouts for connection resiliency (Disconnected: 15s, Failed: 30s, KeepAlive: 2s)
	settingEngine.SetICETimeouts(15*time.Second, 30*time.Second, 2*time.Second)

	// Create and return WebRTC API with configured MediaEngine, InterceptorRegistry, and SettingEngine
	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(interceptorRegistry),
		webrtc.WithSettingEngine(settingEngine),
	)

	return api, nil
}

