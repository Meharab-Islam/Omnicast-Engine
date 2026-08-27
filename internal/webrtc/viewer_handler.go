package webrtc

import (
	"errors"
	"fmt"
	"log"
	"time"

	"omnicast/internal/models"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

// RoomManager defines the interface to lookup rooms without creating circular imports
type RoomManager interface {
	GetRoom(roomID string) (*models.Room, bool)
}

// HandleViewerConnection creates a new PeerConnection for a viewer, fetches the room from RoomManager,
// and attaches the host's VideoTrack and AudioTrack to the viewer's PeerConnection using AddTrack.
func HandleViewerConnection(api *webrtc.API, rm RoomManager, roomID string, config webrtc.Configuration) (*webrtc.PeerConnection, error) {
	if api == nil {
		return nil, errors.New("webrtc api instance is required")
	}
	if rm == nil {
		return nil, errors.New("room manager is required")
	}

	// Lookup room by roomID
	room, exists := rm.GetRoom(roomID)
	if !exists || room == nil {
		return nil, errors.New("room not found: " + roomID)
	}

	return HandleViewerConnectionForRoom(api, room, config)
}

// HandleViewerConnectionForRoom attaches a viewer's PeerConnection directly to the given Room
func HandleViewerConnectionForRoom(api *webrtc.API, room *models.Room, config webrtc.Configuration) (*webrtc.PeerConnection, error) {
	if api == nil {
		return nil, errors.New("webrtc api instance is required")
	}
	if room == nil {
		return nil, errors.New("room is required")
	}

	// Select optimal video track for viewer (Medium 'h', then Low 'q', then Full 'f', then room.VideoTrack)
	videoTrack := room.GetDefaultViewerVideoTrack()
	audioTrack := room.AudioTrack
	coHosts := room.GetAllCoHostTracks()

	// If host media has not published yet, return error gracefully
	if videoTrack == nil && audioTrack == nil && len(coHosts) == 0 {
		return nil, fmt.Errorf("host media not ready")
	}

	// Create a new PeerConnection for the viewer
	peerConnection, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}

	viewerID := fmt.Sprintf("viewer_%d", time.Now().UnixNano())

	var switcher *TrackSwitcher
	if videoTrack != nil {
		// Create dedicated egress TrackLocalStaticRTP for this viewer
		viewerVideoTrack, trackErr := webrtc.NewTrackLocalStaticRTP(
			videoTrack.Codec(),
			videoTrack.ID(),
			videoTrack.StreamID(),
		)
		if trackErr != nil {
			log.Printf("Failed to create TrackLocalStaticRTP for viewer: %v\n", trackErr)
			_ = peerConnection.Close()
			return nil, trackErr
		}

		// Initialize TrackSwitcher with initial layer 'f' (High / Full Resolution HD)
		initialLayer := LayerHigh
		if room.GetVideoTrackByRID(LayerHigh) == nil {
			if room.GetVideoTrackByRID(LayerMedium) != nil {
				initialLayer = LayerMedium
			} else if room.GetVideoTrackByRID(LayerLow) != nil {
				initialLayer = LayerLow
			}
		}

		switcher = NewTrackSwitcher(viewerVideoTrack, initialLayer)
		viewerID = viewerVideoTrack.ID()
		if peerConnection != nil {
			if remoteDesc := peerConnection.RemoteDescription(); remoteDesc != nil && remoteDesc.SDP != "" {
				viewerID = remoteDesc.SDP
			}
		}
		if room != nil {
			room.RegisterTrackSwitcher(viewerID, switcher)
		}

		videoSender, addErr := peerConnection.AddTrack(viewerVideoTrack)
		if addErr != nil {
			log.Printf("Failed to add video track to viewer PeerConnection (Room %s): %v\n", room.RoomID, addErr)
			if room != nil {
				room.UnregisterTrackSwitcher(viewerID)
			}
			_ = peerConnection.Close()
			return nil, addErr
		}

		// 1. Send immediate Keyframe PLI request on track attachment
		if room != nil {
			room.SendPLIImmediate()
		}

		// Read incoming RTCP feedback from viewer (PLI/FIR/NACK/REMB) in background goroutine with ABR auto-switching
		abr := NewABRController()
		go func() {
			rtcpBuf := make([]byte, 1500)

			for {
				if videoSender == nil {
					return
				}
				n, _, rtcpErr := videoSender.Read(rtcpBuf)
				if rtcpErr != nil {
					if room != nil {
						room.UnregisterTrackSwitcher(viewerID)
					}
					if switcher != nil {
						switcher.Close()
					}
					return
				}
				pkts, unmarshalErr := rtcp.Unmarshal(rtcpBuf[:n])
				if unmarshalErr == nil {
					for _, pkt := range pkts {
						switch p := pkt.(type) {
						case *rtcp.ReceiverEstimatedMaximumBitrate:
							estimatedBps := uint64(p.Bitrate)
							optimalLayer := abr.EvaluateLayer(estimatedBps, 0.0)
							if switcher != nil && switcher.GetCurrentLayer() != optimalLayer {
								log.Printf("[ABR Auto-Switch] Room %s: Viewer switching layer %s -> %s (Bitrate: %d bps)\n",
									room.RoomID, switcher.GetCurrentLayer(), optimalLayer, estimatedBps)
								switcher.SwitchLayer(optimalLayer)
								if room != nil {
									room.SendPLIImmediate()
								}
							}
						case *rtcp.TransportLayerNack:
							if switcher != nil && switcher.GetCurrentLayer() == LayerHigh {
								log.Printf("[ABR Packet Loss] Room %s: NACK detected, downgrading to Medium '%s'\n", room.RoomID, LayerMedium)
								switcher.SwitchLayer(LayerMedium)
								if room != nil {
									room.SendPLIImmediate()
								}
							}
						case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
							// Direct RTCP PLI/FIR routing from viewer -> host/publishers for instant keyframe regeneration
							if room != nil {
								room.SendPLIImmediate()
							}
						}
					}
				} else if room != nil {
					// Fallback on any raw RTCP feedback
					room.SendPLIImmediate()
				}
			}
		}()
		log.Printf("Attached TrackSwitcher video track to viewer for Room: %s (Track ID: %s)\n", room.RoomID, viewerVideoTrack.ID())
	}

	// Add host's AudioTrack if available
	if room.AudioTrack != nil {
		audioSender, trackErr := peerConnection.AddTrack(room.AudioTrack)
		if trackErr != nil {
			log.Printf("Failed to add audio track to viewer PeerConnection (Room %s): %v\n", room.RoomID, trackErr)
			_ = peerConnection.Close()
			return nil, trackErr
		}

		// Read incoming RTCP feedback from viewer for audio
		go func() {
			rtcpBufPtr := GetRTPBuffer()
			defer PutRTPBuffer(rtcpBufPtr)
			rtcpBuf := *rtcpBufPtr

			for {
				if _, _, rtcpErr := audioSender.Read(rtcpBuf); rtcpErr != nil {
					return
				}
			}
		}()
		log.Printf("Attached host audio track to viewer for Room: %s\n", room.RoomID)
	}

	// Add existing Co-Host tracks if any co-hosts are already active in the room
	for coHostID, coHostMedia := range room.GetAllCoHostTracks() {
		if coHostMedia != nil {
			if coHostMedia.VideoTrack != nil {
				if _, err := peerConnection.AddTrack(coHostMedia.VideoTrack); err != nil {
					log.Printf("Failed to attach co-host %s video track to viewer (Room %s): %v\n", coHostID, room.RoomID, err)
				} else {
					log.Printf("Attached existing co-host %s video track to viewer for Room: %s\n", coHostID, room.RoomID)
				}
			}
			if coHostMedia.AudioTrack != nil {
				if _, err := peerConnection.AddTrack(coHostMedia.AudioTrack); err != nil {
					log.Printf("Failed to attach co-host %s audio track to viewer (Room %s): %v\n", coHostID, room.RoomID, err)
				} else {
					log.Printf("Attached existing co-host %s audio track to viewer for Room: %s\n", coHostID, room.RoomID)
				}
			}
		}
	}

	// 3. Trigger immediate multi-burst PLI on ICE connected state to guarantee instantaneous keyframe delivery & eliminate datamoshing
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log.Printf("Viewer ICE Connection State (Room %s): %s\n", room.RoomID, connectionState.String())
		if connectionState == webrtc.ICEConnectionStateConnected {
			if room != nil {
				// Immediate initial PLI packet
				room.SendPLIImmediate()
				// Send high-frequency rapid bursts (100ms, 150ms, 250ms, 400ms, 600ms) to ensure
				// the publisher's camera encoder emits an IDR I-Frame at the exact instant the viewer's decoder initializes
				go func() {
					intervals := []time.Duration{
						100 * time.Millisecond,
						150 * time.Millisecond,
						250 * time.Millisecond,
						400 * time.Millisecond,
						600 * time.Millisecond,
					}
					for _, d := range intervals {
						time.Sleep(d)
						if room != nil {
							room.SendPLIImmediate()
						}
					}
				}()
			}
		} else if connectionState == webrtc.ICEConnectionStateFailed || connectionState == webrtc.ICEConnectionStateClosed {
			if switcher != nil {
				switcher.Close()
			}
		}
	})

	// 4. Attach OnTrack listener in case this viewer upgrades to a publisher / co-host
	peerConnection.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("Viewer/CoHost %s incoming track [Kind: %s, ID: %s, SSRC: %d, MimeType: %s]\n",
			viewerID, remoteTrack.Kind().String(), remoteTrack.ID(), remoteTrack.SSRC(), remoteTrack.Codec().MimeType)

		localTrack, trackErr := webrtc.NewTrackLocalStaticRTP(
			remoteTrack.Codec().RTPCodecCapability,
			remoteTrack.ID(),
			remoteTrack.StreamID(),
		)
		if trackErr != nil {
			log.Printf("Failed to create TrackLocalStaticRTP for viewer/cohost %s: %v\n", viewerID, trackErr)
			return
		}

		if remoteTrack.Kind() == webrtc.RTPCodecTypeVideo {
			room.SetCoHostTrack(viewerID, localTrack)
		} else if remoteTrack.Kind() == webrtc.RTPCodecTypeAudio {
			room.SetCoHostAudioTrack(viewerID, localTrack)
		}

		// Forward incoming RTP continuously
		go func() {
			bufPtr := GetRTPBuffer()
			defer PutRTPBuffer(bufPtr)
			buf := *bufPtr

			for {
				n, _, readErr := remoteTrack.Read(buf)
				if readErr != nil {
					return
				}
				if _, writeErr := localTrack.Write(buf[:n]); writeErr != nil {
					return
				}
			}
		}()
	})

	return peerConnection, nil
}

// EstimateViewerBandwidth (Dynacast helper stub): reads incoming RTCP feedback (REMB/TWCC)
// from the viewer's RTPSender and estimates available bandwidth for future dynamic track switching (Dynacast).
func EstimateViewerBandwidth(videoSender *webrtc.RTPSender, room *models.Room, onBitrateEstimate func(bitrateBps uint64)) {
	if videoSender == nil {
		return
	}
	go func() {
		rtcpBufPtr := GetRTPBuffer()
		defer PutRTPBuffer(rtcpBufPtr)
		rtcpBuf := *rtcpBufPtr

		for {
			n, _, rtcpErr := videoSender.Read(rtcpBuf)
			if rtcpErr != nil {
				return
			}
			pkts, err := rtcp.Unmarshal(rtcpBuf[:n])
			if err != nil {
				continue
			}
			for _, pkt := range pkts {
				switch p := pkt.(type) {
				case *rtcp.ReceiverEstimatedMaximumBitrate:
					estimatedBps := uint64(p.Bitrate)
					log.Printf("[Dynacast] Viewer REMB estimated bandwidth: %d bps (%.2f kbps)\n",
						estimatedBps, float64(estimatedBps)/1000.0)
					if onBitrateEstimate != nil {
						onBitrateEstimate(estimatedBps)
					}
				case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
					if room != nil {
						room.SendPLIThrottled(1500 * time.Millisecond)
					}
				}
			}
		}
	}()
}
