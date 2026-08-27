package webrtc

import (
	"errors"
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

	// Create a new PeerConnection for the viewer
	peerConnection, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}

	// Select optimal video track for viewer (Medium 'h', then Low 'q', then Full 'f', then room.VideoTrack)
	videoTrack := room.GetDefaultViewerVideoTrack()

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

		// Initialize TrackSwitcher with initial layer 'h' (Medium)
		initialLayer := LayerMedium
		if room.GetVideoTrackByRID(LayerMedium) == nil {
			if room.GetVideoTrackByRID(LayerLow) != nil {
				initialLayer = LayerLow
			} else {
				initialLayer = LayerHigh
			}
		}

		switcher = NewTrackSwitcher(viewerVideoTrack, initialLayer)
		viewerID := peerConnection.RemoteDescription().SDP
		if viewerID == "" {
			viewerID = viewerVideoTrack.ID()
		}
		room.RegisterTrackSwitcher(viewerID, switcher)

		videoSender, addErr := peerConnection.AddTrack(viewerVideoTrack)
		if addErr != nil {
			log.Printf("Failed to add video track to viewer PeerConnection (Room %s): %v\n", room.RoomID, addErr)
			room.UnregisterTrackSwitcher(viewerID)
			_ = peerConnection.Close()
			return nil, addErr
		}

		// 1. Send initial debounced PLI on track attachment
		room.SendPLIThrottled(1500 * time.Millisecond)

		// Read incoming RTCP feedback from viewer (PLI/FIR/NACK/REMB) in background goroutine with ABR auto-switching
		abr := NewABRController()
		go func() {
			rtcpBufPtr := GetRTPBuffer()
			defer PutRTPBuffer(rtcpBufPtr)
			rtcpBuf := *rtcpBufPtr

			for {
				n, _, rtcpErr := videoSender.Read(rtcpBuf)
				if rtcpErr != nil {
					room.UnregisterTrackSwitcher(viewerID)
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
								room.SendPLIThrottled(1000 * time.Millisecond)
							}
						case *rtcp.TransportLayerNack:
							if switcher != nil && switcher.GetCurrentLayer() == LayerHigh {
								log.Printf("[ABR Packet Loss] Room %s: NACK detected, downgrading to Medium '%s'\n", room.RoomID, LayerMedium)
								switcher.SwitchLayer(LayerMedium)
								room.SendPLIThrottled(1000 * time.Millisecond)
							}
						case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
							room.SendPLIThrottled(1500 * time.Millisecond)
						}
					}
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

	// 3. Trigger PLI on ICE connection state change and PeerConnection state change
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log.Printf("Viewer ICE Connection State (Room %s): %s\n", room.RoomID, connectionState.String())
		if connectionState == webrtc.ICEConnectionStateConnected || connectionState == webrtc.ICEConnectionStateChecking {
			room.SendPLIThrottled(1500 * time.Millisecond)
		}
	})

	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("Viewer PeerConnection State (Room %s): %s\n", room.RoomID, state.String())
		if state == webrtc.PeerConnectionStateConnected {
			room.SendPLIThrottled(1500 * time.Millisecond)
		}
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
