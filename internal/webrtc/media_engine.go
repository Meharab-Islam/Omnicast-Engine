package webrtc

import (
	"errors"
	"io"
	"log"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/nack"
	"github.com/pion/interceptor/pkg/twcc"
	"github.com/pion/rtcp"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
)

// InitOmnicastMediaEngine configures strict Codec Enforcement (VP8 and Opus ONLY),
// TWCC header extensions, and QoS interceptors for the Omnicast SFU.
func InitOmnicastMediaEngine() (*webrtc.MediaEngine, *interceptor.Registry, error) {
	mediaEngine := &webrtc.MediaEngine{}

	// 1. 🎵 Strict Codec Enforcement: Register Opus Audio Codec ONLY
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
		return nil, nil, err
	}

	// 2. 🎬 Strict Codec Enforcement: Register VP8 Video Codec ONLY (Reject H.264 to prevent mobile hardware encoder crashes)
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
		return nil, nil, err
	}

	// 3. 📡 Header Extensions: Register TWCC & Simulcast extensions
	for _, uri := range []string{
		sdp.SDESMidURI,
		sdp.SDESRTPStreamIDURI,
		sdp.SDESRepairRTPStreamIDURI,
		sdp.TransportCCURI,
		"http://www.webrtc.org/experiments/rtp-hdrext/abs-send-time",
	} {
		if err := mediaEngine.RegisterHeaderExtension(
			webrtc.RTPHeaderExtensionCapability{URI: uri},
			webrtc.RTPCodecTypeVideo,
		); err != nil {
			return nil, nil, err
		}
	}

	// 4. ⚡ InterceptorRegistry: Configure TWCC Bandwidth Estimation & Aggressive NACK
	interceptorRegistry := &interceptor.Registry{}

	// Aggressive NACK Generator (20ms check interval, max 3 NACKs before firing PLI)
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

	// TWCC Sender Interceptor: Tracks packet arrivals and calculates real-time bandwidth
	if senderFactory, err := twcc.NewSenderInterceptor(); err == nil && senderFactory != nil {
		interceptorRegistry.Add(senderFactory)
	}

	// TWCC Header Extension Interceptor
	if headerFactory, err := twcc.NewHeaderExtensionInterceptor(); err == nil && headerFactory != nil {
		interceptorRegistry.Add(headerFactory)
	}

	if err := webrtc.ConfigureTWCCHeaderExtensionSender(mediaEngine, interceptorRegistry); err != nil {
		return nil, nil, err
	}

	if err := webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry); err != nil {
		return nil, nil, err
	}

	return mediaEngine, interceptorRegistry, nil
}

// NewOmnicastWebRTCAPI returns a fully configured *webrtc.API combining
// the SettingEngine (from Step 1), MediaEngine, and InterceptorRegistry.
func NewOmnicastWebRTCAPI(settingEngine webrtc.SettingEngine) (*webrtc.API, error) {
	mediaEngine, interceptorRegistry, err := InitOmnicastMediaEngine()
	if err != nil {
		return nil, err
	}

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(interceptorRegistry),
		webrtc.WithSettingEngine(settingEngine),
	)

	return api, nil
}

// RouteRTCPFeedback reads RTCP feedback from a viewer's sender/receiver, intercepts PLI (Picture Loss Indication)
// and FIR (Full Intra Request) packets, and instantly forwards them to the active broadcaster's PeerConnection to force a Keyframe.
func RouteRTCPFeedback(viewerSender *webrtc.RTPSender, broadcasterPC *webrtc.PeerConnection, broadcasterSSRC uint32) {
	if viewerSender == nil || broadcasterPC == nil {
		return
	}

	go func() {
		for {
			pkts, _, err := viewerSender.ReadRTCP()
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
					return
				}
				return
			}

			for _, pkt := range pkts {
				switch p := pkt.(type) {
				case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
					// Instant Keyframe Request: Forward PLI directly to the broadcaster
					if broadcasterPC.ConnectionState() != webrtc.PeerConnectionStateClosed {
						pli := &rtcp.PictureLossIndication{MediaSSRC: broadcasterSSRC}
						if pPLI, ok := p.(*rtcp.PictureLossIndication); ok && pPLI.MediaSSRC != 0 {
							pli.MediaSSRC = pPLI.MediaSSRC
						}
						_ = broadcasterPC.WriteRTCP([]rtcp.Packet{pli})
						log.Printf("[QoS Router] Instantly relayed RTCP PLI keyframe request to broadcaster (SSRC: %d)\n", pli.MediaSSRC)
					}
				}
			}
		}
	}()
}
