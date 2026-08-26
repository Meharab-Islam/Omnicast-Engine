package webrtc

import (
	"errors"
	"log"
	"time"

	"live-media-server/internal/models"

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

	// Select optimal video track for viewer (Basic Adaptive Logic: prefer Medium 'h', fallback to Low 'q', High 'f', or room.VideoTrack)
	videoTrack := room.GetDefaultViewerVideoTrack()

	// Determine assigned layer RID ('h', 'q', 'f', or 'default') and matching SSRC
	assignedRID := "default"
	if videoTrack != nil {
		if room.GetVideoTrackByRID("h") == videoTrack {
			assignedRID = "h"
		} else if room.GetVideoTrackByRID("q") == videoTrack {
			assignedRID = "q"
		} else if room.GetVideoTrackByRID("f") == videoTrack {
			assignedRID = "f"
		}
	}

	assignedSSRC := room.GetVideoTrackSSRC(assignedRID)

	// Helper to send immediate PLI (Picture Loss Indication) keyframe request to host for the assigned track
	sendPLIToHost := func() {
		hostPC := room.GetHostPeerConnection()
		ssrc := assignedSSRC
		if ssrc == 0 {
			ssrc = room.GetHostVideoSSRC()
		}
		if hostPC != nil && videoTrack != nil && hostPC.ConnectionState() != webrtc.PeerConnectionStateClosed && ssrc != 0 {
			err := hostPC.WriteRTCP([]rtcp.Packet{
				&rtcp.PictureLossIndication{MediaSSRC: ssrc},
			})
			if err != nil {
				log.Printf("Failed to write PLI RTCP to host (Room %s, RID '%s', SSRC %d): %v\n", room.RoomID, assignedRID, ssrc, err)
			} else {
				log.Printf("Sent PLI (Keyframe request) to Host for Room: %s (RID: '%s', SSRC: %d)\n", room.RoomID, assignedRID, ssrc)
			}
		}
	}

	if videoTrack != nil {
		videoSender, trackErr := peerConnection.AddTrack(videoTrack)
		if trackErr != nil {
			log.Printf("Failed to add video track to viewer PeerConnection (Room %s): %v\n", room.RoomID, trackErr)
			_ = peerConnection.Close()
			return nil, trackErr
		}

		// 1. Send immediate PLI on track attachment
		sendPLIToHost()

		// 2. Start a background Goroutine with time.NewTicker(3 * time.Second) to request Keyframe
		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()

			for range ticker.C {
				if peerConnection.ConnectionState() == webrtc.PeerConnectionStateClosed {
					return
				}
				sendPLIToHost()
			}
		}()

		// Read incoming RTCP feedback from viewer (PLI/FIR/NACK/REMB) in background goroutine
		go func() {
			rtcpBuf := make([]byte, 1500)
			for {
				n, _, rtcpErr := videoSender.Read(rtcpBuf)
				if rtcpErr != nil {
					return
				}
				pkts, unmarshalErr := rtcp.Unmarshal(rtcpBuf[:n])
				if unmarshalErr == nil {
					for _, pkt := range pkts {
						switch p := pkt.(type) {
						case *rtcp.ReceiverEstimatedMaximumBitrate:
							log.Printf("[Dynacast] Viewer REMB estimated bitrate for Room %s: %d bps (%.2f kbps)\n",
								room.RoomID, uint64(p.Bitrate), float64(p.Bitrate)/1000.0)
						case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
							sendPLIToHost()
						}
					}
				} else {
					sendPLIToHost()
				}
			}
		}()
		log.Printf("Attached host video track to viewer for Room: %s (Track ID: %s)\n", room.RoomID, videoTrack.ID())
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
			rtcpBuf := make([]byte, 1500)
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
			sendPLIToHost()
		}
	})

	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("Viewer PeerConnection State (Room %s): %s\n", room.RoomID, state.String())
		if state == webrtc.PeerConnectionStateConnected {
			sendPLIToHost()
		}
	})

	return peerConnection, nil
}

// EstimateViewerBandwidth (Dynacast helper stub): reads incoming RTCP feedback (REMB/TWCC)
// from the viewer's RTPSender and estimates available bandwidth for future dynamic track switching (Dynacast).
func EstimateViewerBandwidth(videoSender *webrtc.RTPSender, onBitrateEstimate func(bitrateBps uint64)) {
	if videoSender == nil {
		return
	}
	go func() {
		rtcpBuf := make([]byte, 1500)
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
				}
			}
		}
	}()
}
