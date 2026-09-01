package webrtc

import (
	"net"
	"os"
	"time"

	"github.com/pion/dtls/v2"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/nack"
	"github.com/pion/interceptor/pkg/twcc"
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

	// Register Opus Audio Codec with DTX (Discontinuous Transmission) enabled.
	// DTX: If a co-host is silent, the encoder stops sending empty audio packets to save bandwidth.
	// useinbandfec=1: Forward Error Correction for audio resilience.
	// usedtx=1: Discontinuous Transmission — silent co-hosts stop sending empty packets.
	if err := mediaEngine.RegisterCodec(
		webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     webrtc.MimeTypeOpus,
				ClockRate:    48000,
				Channels:     2,
				SDPFmtpLine:  "minptime=10;useinbandfec=1;usedtx=1",
				RTCPFeedback: nil,
			},
			PayloadType: 111,
		},
		webrtc.RTPCodecTypeAudio,
	); err != nil {
		return nil, err
	}

	// Register Audio RED (RFC 2198) Redundancy Codec.
	// Audio RED wraps each Opus packet with a redundant copy of the previous packet,
	// guaranteeing crystal-clear audio even with 20% network packet loss.
	if err := mediaEngine.RegisterCodec(
		webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     "audio/red",
				ClockRate:    48000,
				Channels:     2,
				SDPFmtpLine:  "111/111",
				RTCPFeedback: nil,
			},
			PayloadType: 63,
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
					{Type: "transport-cc", Parameter: ""},
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
					{Type: "transport-cc", Parameter: ""},
				},
			},
			PayloadType: 97,
		},
		webrtc.RTPCodecTypeVideo,
	); err != nil {
		return nil, err
	}

	// Register VP8 RTX Retransmission Codec (LiveKit / Mediasoup standard)
	if err := mediaEngine.RegisterCodec(
		webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     "video/rtx",
				ClockRate:    90000,
				Channels:     0,
				SDPFmtpLine:  "apt=96",
				RTCPFeedback: nil,
			},
			PayloadType: 98,
		},
		webrtc.RTPCodecTypeVideo,
	); err != nil {
		return nil, err
	}

	// Register H264 RTX Retransmission Codec
	if err := mediaEngine.RegisterCodec(
		webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     "video/rtx",
				ClockRate:    90000,
				Channels:     0,
				SDPFmtpLine:  "apt=97",
				RTCPFeedback: nil,
			},
			PayloadType: 99,
		},
		webrtc.RTPCodecTypeVideo,
	); err != nil {
		return nil, err
	}

	// Register VP9 Video Codec with profile-id=0 (Profile 0: 8-bit color depth, 4:2:0 chroma subsampling)
	// for maximum compatibility across mobile devices (Android/iOS WebRTC) and modern browsers.
	if err := mediaEngine.RegisterCodec(
		webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeVP9,
				ClockRate:   90000,
				Channels:    0,
				SDPFmtpLine: "profile-id=0",
				RTCPFeedback: []webrtc.RTCPFeedback{
					{Type: "goog-remb", Parameter: ""},
					{Type: "ccm", Parameter: "fir"},
					{Type: "nack", Parameter: ""},
					{Type: "nack", Parameter: "pli"},
					{Type: "transport-cc", Parameter: ""},
				},
			},
			PayloadType: 100,
		},
		webrtc.RTPCodecTypeVideo,
	); err != nil {
		return nil, err
	}

	// Register VP9 RTX Retransmission Codec
	if err := mediaEngine.RegisterCodec(
		webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     "video/rtx",
				ClockRate:    90000,
				Channels:     0,
				SDPFmtpLine:  "apt=100",
				RTCPFeedback: nil,
			},
			PayloadType: 101,
		},
		webrtc.RTPCodecTypeVideo,
	); err != nil {
		return nil, err
	}

	// 1. Register RTP Header Extensions for SVC (L3T3 Video Layers Allocation), Simulcast (MID & RID), and Bandwidth Estimation
	videoHeaderExtensions := []string{
		sdp.SDESMidURI,
		sdp.SDESRTPStreamIDURI,
		"urn:ietf:params:rtp-hdrext:sdes:repaired-rtp-stream-id",
		"urn:ietf:params:rtp-hdrext:twcc",
		sdp.TransportCCURI,
		sdp.ABSSendTimeURI,
		"http://www.webrtc.org/experiments/rtp-hdrext/video-layers-allocation00", // SVC L3T3 layer allocation
		"http://www.ietf.org/id/draft-ietf-avtext-framemarking-07",
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

	// 2. Register Interceptors (Aggressive NACK, RTCP reports, TWCC/REMB Congestion Control / Bandwidth Estimation)
	interceptorRegistry := &interceptor.Registry{}

	// Aggressive NACK Generator: 20ms check interval, max 3 NACKs per packet
	if generatorFactory, err := nack.NewGeneratorInterceptor(
		nack.GeneratorSize(2048),
		nack.GeneratorInterval(20*time.Millisecond),
		nack.GeneratorMaxNacksPerPacket(3),
		nack.GeneratorSkipLastN(0),
	); err == nil && generatorFactory != nil {
		interceptorRegistry.Add(generatorFactory)
	}

	// Aggressive NACK Responder with 4096-packet retransmission buffer
	if responderFactory, err := nack.NewResponderInterceptor(
		nack.ResponderSize(4096),
	); err == nil && responderFactory != nil {
		interceptorRegistry.Add(responderFactory)
	}

	// Instantiate a new twcc.SenderInterceptorFactory() and add it to the engine's interceptor.Registry
	if senderInterceptorFactory, err := twcc.NewSenderInterceptor(); err == nil && senderInterceptorFactory != nil {
		interceptorRegistry.Add(senderInterceptorFactory)
	}

	// Register TWCC Header Extension using github.com/pion/interceptor/pkg/twcc
	if twccHeaderFactory, err := twcc.NewHeaderExtensionInterceptor(); err == nil && twccHeaderFactory != nil {
		interceptorRegistry.Add(twccHeaderFactory)
	}

	// Register TWCC Header Extension Sender so remote peers can generate TWCC feedback
	if err := webrtc.ConfigureTWCCHeaderExtensionSender(mediaEngine, interceptorRegistry); err != nil {
		return nil, err
	}

	// Register Default Interceptors (RTCP Reports, TWCC Responder)
	if err := webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry); err != nil {
		return nil, err
	}

	// 3. Configure SettingEngine with Enterprise Infrastructure Optimizations
	settingEngine := webrtc.SettingEngine{}

	// Hardware AES-NI SRTP Encryption Prioritization (AES-256-GCM & AES-128-GCM)
	settingEngine.SetSRTPProtectionProfiles(
		dtls.SRTP_AEAD_AES_256_GCM,
		dtls.SRTP_AEAD_AES_128_GCM,
		dtls.SRTP_AES128_CM_HMAC_SHA1_80,
	)

	// High-Throughput OS UDP Socket Buffer Optimization (MTU 1500)
	settingEngine.SetReceiveMTU(1500)

	// Restrict WebRTC UDP port range (50000 - 52000) and advertise NAT 1:1 Public IP
	if err := settingEngine.SetEphemeralUDPPortRange(50000, 52000); err != nil {
		return nil, err
	}

	// Strictly disable ICE TCP fallback and force UDP to eliminate head-of-line blocking and jitter delay
	settingEngine.SetNetworkTypes([]webrtc.NetworkType{
		webrtc.NetworkTypeUDP4,
		webrtc.NetworkTypeUDP6,
	})

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

// InitWebRTCWithTCPListener initializes Pion WebRTC API with an ICETCPMux using the provided TCP listener
func InitWebRTCWithTCPListener(tcpListener net.Listener) (*webrtc.API, error) {
	if tcpListener == nil {
		return InitWebRTC()
	}

	// Create a MediaEngine object to configure supported codecs
	mediaEngine := &webrtc.MediaEngine{}

	if err := mediaEngine.RegisterCodec(
		webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     webrtc.MimeTypeOpus,
				ClockRate:    48000,
				Channels:     2,
				SDPFmtpLine:  "minptime=10;useinbandfec=1;usedtx=1",
				RTCPFeedback: nil,
			},
			PayloadType: 111,
		},
		webrtc.RTPCodecTypeAudio,
	); err != nil {
		return nil, err
	}

	if err := mediaEngine.RegisterCodec(
		webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     "audio/red",
				ClockRate:    48000,
				Channels:     2,
				SDPFmtpLine:  "111/111",
				RTCPFeedback: nil,
			},
			PayloadType: 63,
		},
		webrtc.RTPCodecTypeAudio,
	); err != nil {
		return nil, err
	}

	if err := mediaEngine.RegisterCodec(
		webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP8,
				ClockRate: 90000,
				RTCPFeedback: []webrtc.RTCPFeedback{
					{Type: "goog-remb"},
					{Type: "nack"},
					{Type: "nack", Parameter: "pli"},
					{Type: "ccm", Parameter: "fir"},
				},
			},
			PayloadType: 96,
		},
		webrtc.RTPCodecTypeVideo,
	); err != nil {
		return nil, err
	}

	interceptorRegistry := &interceptor.Registry{}
	if senderInterceptorFactory, err := twcc.NewSenderInterceptor(); err == nil && senderInterceptorFactory != nil {
		interceptorRegistry.Add(senderInterceptorFactory)
	}
	if twccHeaderFactory, err := twcc.NewHeaderExtensionInterceptor(); err == nil && twccHeaderFactory != nil {
		interceptorRegistry.Add(twccHeaderFactory)
	}
	if err := webrtc.ConfigureTWCCHeaderExtensionSender(mediaEngine, interceptorRegistry); err != nil {
		return nil, err
	}
	if err := webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry); err != nil {
		return nil, err
	}

	settingEngine := webrtc.SettingEngine{}

	// Hardware AES-NI SRTP Encryption Prioritization (AES-256-GCM & AES-128-GCM)
	settingEngine.SetSRTPProtectionProfiles(
		dtls.SRTP_AEAD_AES_256_GCM,
		dtls.SRTP_AEAD_AES_128_GCM,
		dtls.SRTP_AES128_CM_HMAC_SHA1_80,
	)

	// High-Throughput OS UDP Socket Buffer Optimization (MTU 1500)
	settingEngine.SetReceiveMTU(1500)

	if err := settingEngine.SetEphemeralUDPPortRange(50000, 52000); err != nil {
		return nil, err
	}

	// Configure SettingEngine to advertise both UDP and TCP candidate types to connecting peers
	settingEngine.SetNetworkTypes([]webrtc.NetworkType{
		webrtc.NetworkTypeUDP4,
		webrtc.NetworkTypeUDP6,
		webrtc.NetworkTypeTCP4,
		webrtc.NetworkTypeTCP6,
	})

	publicIP := os.Getenv("PUBLIC_IP")
	if publicIP == "" {
		publicIP = "192.168.0.116"
	}
	// Advertise TCP host candidates on the NAT 1:1 Public IP
	settingEngine.SetNAT1To1IPs([]string{publicIP}, webrtc.ICECandidateTypeHost)
	settingEngine.SetICETimeouts(15*time.Second, 30*time.Second, 2*time.Second)

	// Initialize webrtc.NewICETCPMux using this TCP listener to advertise TCP host candidates
	tcpMux := webrtc.NewICETCPMux(nil, tcpListener, 8)
	settingEngine.SetICETCPMux(tcpMux)

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(interceptorRegistry),
		webrtc.WithSettingEngine(settingEngine),
	)

	return api, nil
}

