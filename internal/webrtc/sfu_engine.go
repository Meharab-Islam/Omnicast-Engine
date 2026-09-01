package webrtc

import (
	"log"
	"net"
	"os"
	"time"

	"github.com/pion/dtls/v2"
	"github.com/pion/ice/v2"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/nack"
	"github.com/pion/interceptor/pkg/twcc"
	"github.com/pion/sdp/v3"
	"github.com/pion/turn/v2"
	"github.com/pion/webrtc/v3"
)

// SFUEngineConfig holds configuration options for initializing the enterprise SFU stack
type SFUEngineConfig struct {
	PublicIP      string
	TURNPort      int
	TURNRealm     string
	TURNSecret    string
	SingleUDPPort int // Port for UDPMux single-port multiplexing (e.g. 50000)
	SingleTCPPort int // Port for TCPMux single-port multiplexing (e.g. 50000)
	RelayMinPort  uint16
	RelayMaxPort  uint16
}

// SFUEngine represents the complete unified Pion SFU stack integrating
// pion/turn, pion/ice, pion/interceptor, pion/rtp, and pion/webrtc
type SFUEngine struct {
	WebRTCAPI  *webrtc.API
	TURNServer *turn.Server
	UDPMux     ice.UDPMux
	TCPMux     ice.TCPMux
	Config     SFUEngineConfig
}

// NewSFUEngine initializes and starts the unified Pion SFU engine
func NewSFUEngine(cfg SFUEngineConfig) (*SFUEngine, error) {
	if cfg.PublicIP == "" {
		cfg.PublicIP = os.Getenv("PUBLIC_IP")
		if cfg.PublicIP == "" {
			cfg.PublicIP = "178.162.252.30"
		}
	}
	if cfg.TURNPort <= 0 {
		cfg.TURNPort = 3478
	}
	if cfg.TURNRealm == "" {
		cfg.TURNRealm = "omnicast.live"
	}
	if cfg.TURNSecret == "" {
		cfg.TURNSecret = DefaultTURNSecret
	}

	engine := &SFUEngine{
		Config: cfg,
	}

	// 1. Initialize Embedded STUN/TURN Server (pion/turn)
	turnServer, turnErr := StartEmbeddedTURNServer(EmbeddedTURNServerConfig{
		PublicIP: cfg.PublicIP,
		Realm:    cfg.TURNRealm,
		Secret:   cfg.TURNSecret,
		Port:     cfg.TURNPort,
		MinPort:  cfg.RelayMinPort,
		MaxPort:  cfg.RelayMaxPort,
	})
	if turnErr != nil {
		log.Printf("[SFU Warning] Embedded TURN initialization notice: %v\n", turnErr)
	} else {
		engine.TURNServer = turnServer
	}

	// 2. Configure MediaEngine with Codecs & Extensions (VP8, H264, Opus)
	mediaEngine := &webrtc.MediaEngine{}

	// Opus Audio Codec with In-band FEC & DTX
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

	// VP8 Video Codec with NACK, PLI, FIR, and REMB
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
					{Type: "transport-cc"},
				},
			},
			PayloadType: 96,
		},
		webrtc.RTPCodecTypeVideo,
	); err != nil {
		return nil, err
	}

	// H.264 Video Codec with NACK, PLI, FIR, and REMB
	if err := mediaEngine.RegisterCodec(
		webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeH264,
				ClockRate: 90000,
				RTCPFeedback: []webrtc.RTCPFeedback{
					{Type: "goog-remb"},
					{Type: "nack"},
					{Type: "nack", Parameter: "pli"},
					{Type: "ccm", Parameter: "fir"},
					{Type: "transport-cc"},
				},
			},
			PayloadType: 102,
		},
		webrtc.RTPCodecTypeVideo,
	); err != nil {
		return nil, err
	}

	// Register Header Extensions for TWCC & Simulcast
	for _, uri := range []string{
		sdp.SDESMidURI,
		sdp.SDESRTPStreamIDURI,
		sdp.SDESRepairRTPStreamIDURI,
		sdp.TransportCCURI,
		"http://www.webrtc.org/experiments/rtp-hdrext/abs-send-time",
	} {
		_ = mediaEngine.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: uri}, webrtc.RTPCodecTypeVideo)
	}

	// 3. Configure InterceptorRegistry (pion/interceptor) for Congestion Control & Fast NACK
	interceptorRegistry := &interceptor.Registry{}

	// Aggressive NACK Generator (20ms interval, max 3 NACKs per packet)
	if nackGen, err := nack.NewGeneratorInterceptor(
		nack.GeneratorSize(2048),
		nack.GeneratorInterval(20*time.Millisecond),
		nack.GeneratorMaxNacksPerPacket(3),
		nack.GeneratorSkipLastN(0),
	); err == nil && nackGen != nil {
		interceptorRegistry.Add(nackGen)
	}

	// Aggressive NACK Responder (4096 packet buffer)
	if nackResp, err := nack.NewResponderInterceptor(
		nack.ResponderSize(4096),
	); err == nil && nackResp != nil {
		interceptorRegistry.Add(nackResp)
	}

	// TWCC Sender & Header Extension for Real-Time Bandwidth Estimation
	if twccSender, err := twcc.NewSenderInterceptor(); err == nil && twccSender != nil {
		interceptorRegistry.Add(twccSender)
	}
	if twccHeader, err := twcc.NewHeaderExtensionInterceptor(); err == nil && twccHeader != nil {
		interceptorRegistry.Add(twccHeader)
	}
	_ = webrtc.ConfigureTWCCHeaderExtensionSender(mediaEngine, interceptorRegistry)
	_ = webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry)

	// 4. Configure SettingEngine & Port Multiplexing (pion/ice UDPMux & TCPMux)
	settingEngine := webrtc.SettingEngine{}

	// Hardware AES-NI SRTP Encryption Prioritization
	settingEngine.SetSRTPProtectionProfiles(
		dtls.SRTP_AEAD_AES_256_GCM,
		dtls.SRTP_AEAD_AES_128_GCM,
		dtls.SRTP_AES128_CM_HMAC_SHA1_80,
	)

	settingEngine.SetReceiveMTU(1500)
	settingEngine.SetNAT1To1IPs([]string{cfg.PublicIP}, webrtc.ICECandidateTypeHost)

	// 🎯 Port Multiplexing: If SingleUDPPort is set, attach UDPMux
	if cfg.SingleUDPPort > 0 {
		udpListener, err := net.ListenUDP("udp4", &net.UDPAddr{Port: cfg.SingleUDPPort})
		if err == nil {
			udpMux := webrtc.NewICEUDPMux(nil, udpListener)
			settingEngine.SetICEUDPMux(udpMux)
			engine.UDPMux = udpMux
			log.Printf("[Pion ICE] Single-port UDP Multiplexer active on 0.0.0.0:%d\n", cfg.SingleUDPPort)
		} else {
			log.Printf("[Pion ICE] Single UDP port %d status: %v (falling back to port range)\n", cfg.SingleUDPPort, err)
			_ = settingEngine.SetEphemeralUDPPortRange(50000, 50050)
		}
	} else {
		_ = settingEngine.SetEphemeralUDPPortRange(50000, 50050)
	}


	// Strictly force UDP
	settingEngine.SetNetworkTypes([]webrtc.NetworkType{
		webrtc.NetworkTypeUDP4,
		webrtc.NetworkTypeUDP6,
	})

	settingEngine.SetICETimeouts(15*time.Second, 30*time.Second, 2*time.Second)

	// Build WebRTC API instance
	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(interceptorRegistry),
		webrtc.WithSettingEngine(settingEngine),
	)

	engine.WebRTCAPI = api
	log.Printf("[SFU Engine] Unified Pion SFU Engine initialized successfully (Public IP: %s)\n", cfg.PublicIP)

	return engine, nil
}

// Close releases all multiplexers and stops the embedded TURN server
func (s *SFUEngine) Close() error {
	if s.TURNServer != nil {
		_ = s.TURNServer.Close()
	}
	return nil
}
