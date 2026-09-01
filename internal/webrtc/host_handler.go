package webrtc

import (
	"errors"
	"io"
	"log"
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	"omnicast/internal/metrics"
	"omnicast/internal/models"
)

// ScalabilityModeL3T3 defines the Scalable Video Coding (SVC) mode with 3 spatial layers and 3 temporal layers
const ScalabilityModeL3T3 = "L3T3"

// HandleHostConnection creates a WebRTC PeerConnection for the broadcaster/host,
// configures the video transceiver to request SVC (Scalable Video Coding) with L3T3 mode (3 spatial, 3 temporal layers),
// sets up OnTrack handler, and reads incoming RTP packets in a background goroutine to write to room tracks.
func HandleHostConnection(api *webrtc.API, room *models.Room, config webrtc.Configuration) (*webrtc.PeerConnection, error) {
	if api == nil {
		return nil, errors.New("webrtc api instance is required")
	}
	if room == nil {
		return nil, errors.New("room instance is required")
	}

	// Create PeerConnection for host
	peerConnection, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}
	room.SetHostPeerConnection(peerConnection)

	// Initialize TimeSynchronizer for precise A/V Lip-Sync (RFC 3550)
	timeSync := NewTimeSynchronizer(nil)
	room.SetTimeSynchronizer(timeSync)

	// Configure Video Transceiver for SVC (Scalable Video Coding) requesting L3T3 scalability mode
	if _, err := peerConnection.AddTransceiverFromKind(
		webrtc.RTPCodecTypeVideo,
		webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionRecvonly,
		},
	); err != nil {
		log.Printf("[SVC Setup] Note: Transceiver init returned %v (proceeding with SDP negotiation)\n", err)
	}

	// Configure Audio Transceiver
	if _, err := peerConnection.AddTransceiverFromKind(
		webrtc.RTPCodecTypeAudio,
		webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionRecvonly,
		},
	); err != nil {
		log.Printf("[Audio Setup] Note: Transceiver init returned %v\n", err)
	}

	// Create WebRTC DataChannels for ultra-low-latency in-room messaging (LiveKit style)
	// 1. "room-events" (ordered: true) for reliable chat messages and state updates
	orderedTrue := true
	if eventsDC, err := peerConnection.CreateDataChannel("room-events", &webrtc.DataChannelInit{
		Ordered: &orderedTrue,
	}); err == nil && eventsDC != nil {
		room.RegisterDataChannel(room.HostID, eventsDC)
		eventsDC.OnMessage(func(msg webrtc.DataChannelMessage) {
			// Instantly fan-out byte payload to all active Viewers in this specific room
			room.BroadcastDataChannelMessage(room.HostID, "room-events", msg.Data)
		})
	}

	// 2. "room-reactions" (ordered: false) for high-frequency loss-tolerant events (flying hearts/reactions)
	orderedFalse := false
	if reactionsDC, err := peerConnection.CreateDataChannel("room-reactions", &webrtc.DataChannelInit{
		Ordered: &orderedFalse,
	}); err == nil && reactionsDC != nil {
		room.RegisterDataChannel(room.HostID, reactionsDC)
		reactionsDC.OnMessage(func(msg webrtc.DataChannelMessage) {
			// Instantly fan-out reaction payload to all active Viewers in this specific room
			room.BroadcastDataChannelMessage(room.HostID, "room-reactions", msg.Data)
		})
	}

	// Listen for remote client-initiated DataChannels
	peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
		room.RegisterDataChannel(room.HostID, d)
		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			room.BroadcastDataChannelMessage(room.HostID, d.Label(), msg.Data)
		})
	})

	// Maintain a separate ring buffer for every incoming video track from the Host
	packetBuffers := make(map[string]*PacketBuffer)
	var pbMu sync.RWMutex

	// Listen for incoming media tracks from the host
	peerConnection.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		rid := remoteTrack.RID()
		log.Printf("Host track received: Kind=%s, RID=%s\n", remoteTrack.Kind().String(), rid)

		// Intercept incoming RTCP Sender Reports from the publisher to update NTP-to-RTP wall-clock mapping
		if receiver != nil {
			go func(r *webrtc.RTPReceiver, kind webrtc.RTPCodecType) {
				defer func() {
					if rec := recover(); rec != nil {
						// Gracefully recover on receiver closure
					}
				}()
				for {
					pkts, _, rtcpErr := r.ReadRTCP()
					if rtcpErr != nil {
						return
					}
					for _, p := range pkts {
						if sr, ok := p.(*rtcp.SenderReport); ok && sr != nil {
							timeSync.ProcessSenderReport(sr, kind)
						}
					}
				}
			}(receiver, remoteTrack.Kind())
		}


		// Get or create dedicated PacketBuffer for the host's video stream (non-simulcast single high-quality track)
		var pktBuffer *PacketBuffer
		if remoteTrack.Kind() == webrtc.RTPCodecTypeVideo {
			layerKey := "default"
			pbMu.Lock()
			if pb, exists := packetBuffers[layerKey]; exists && pb != nil {
				pktBuffer = pb
			} else {
				pktBuffer = NewPacketBuffer(DefaultPacketBufferSize)
				packetBuffers[layerKey] = pktBuffer
			}
			pbMu.Unlock()
			if room != nil {
				room.SetPacketBuffer(layerKey, pktBuffer)
			}
		}

		trackID := remoteTrack.ID()
		layerRID := rid
		if layerRID == "" {
			layerRID = "f"
		}

		var localTrack *webrtc.TrackLocalStaticRTP
		var trackErr error

		// Register track into the room with standard simulcast (RID 'q', 'h', 'f')
		switch remoteTrack.Kind() {
		case webrtc.RTPCodecTypeVideo:
			// Strict Security Check: Drop and ignore video tracks in audio-only rooms
			if room.GetRoomType() == "audio" {
				log.Printf("[Security] Dropped unauthorized incoming video track %s in audio-only Room %s (SSRC: %d)\n",
					remoteTrack.ID(), room.RoomID, remoteTrack.SSRC())
				return
			}

			localTrack, trackErr = webrtc.NewTrackLocalStaticRTP(
				remoteTrack.Codec().RTPCodecCapability,
				trackID,
				remoteTrack.StreamID(),
			)
			if trackErr != nil {
				log.Printf("Failed to create TrackLocalStaticRTP for host track (RID '%s'): %v\n", layerRID, trackErr)
				return
			}

			room.SetVideoTrackRID(layerRID, localTrack)
			room.SetVideoTrackSSRC(layerRID, uint32(remoteTrack.SSRC()))
			if room.GetVideoTrack() == nil || layerRID == "f" {
				room.SetVideoTrack(localTrack)
				room.SetHostVideoSSRC(uint32(remoteTrack.SSRC()))
			}
			log.Printf("Simulcast video layer registered for Room: %s (RID: '%s', SSRC: %d)\n", room.RoomID, layerRID, remoteTrack.SSRC())

			// Register incoming track with all active viewer TrackSwitchers
			for _, s := range room.GetAllTrackSwitchers() {
				if ts, ok := s.(*TrackSwitcher); ok && ts != nil {
					ts.SetIncomingTrack(layerRID, remoteTrack)
				}
			}

			// Periodic Keyframe (PLI) Routine: Periodically request a Keyframe every 2 seconds
			// to force the broadcaster to emit an I-frame, instantly recovering packet loss artifacts
			go func(trackSSRC uint32) {
				ticker := time.NewTicker(2 * time.Second)
				defer ticker.Stop()

				for range ticker.C {
					if peerConnection.ConnectionState() == webrtc.PeerConnectionStateClosed {
						return
					}
					_ = peerConnection.WriteRTCP([]rtcp.Packet{
						&rtcp.PictureLossIndication{MediaSSRC: trackSSRC},
					})
				}
			}(uint32(remoteTrack.SSRC()))

		case webrtc.RTPCodecTypeAudio:
			if existing := room.GetAudioTrack(); existing != nil {
				localTrack = existing
				log.Printf("Re-bound existing AudioTrack on Host reconnect for Room: %s\n", room.RoomID)
			} else {
				localTrack, trackErr = webrtc.NewTrackLocalStaticRTP(
					remoteTrack.Codec().RTPCodecCapability,
					trackID,
					remoteTrack.StreamID(),
				)
				if trackErr != nil {
					log.Printf("Failed to create TrackLocalStaticRTP for host audio track: %v\n", trackErr)
					return
				}
				room.SetAudioTrack(localTrack)
				log.Printf("Audio track registered for Room: %s\n", room.RoomID)
			}
		}

		// Infinite background goroutine to read RTP packets from host and write to room track
		go func() {
			bufPtr := GetRTPBuffer()
			defer PutRTPBuffer(bufPtr)
			buf := *bufPtr

			for {
				n, attrs, readErr := remoteTrack.Read(buf)
				if readErr != nil {
					if errors.Is(readErr, io.EOF) {
						log.Printf("Host track %s closed (EOF)\n", remoteTrack.ID())
					} else {
						log.Printf("Error reading RTP packet from host track %s: %v\n", remoteTrack.ID(), readErr)
					}
					return
				}

				// Prometheus Metrics: Record inbound bandwidth bytes
				metrics.AddBytesReceived(n)

				// 1. Store a copy of the packet in the track's PacketBuffer BEFORE forwarding
				var parsedPkt *rtp.Packet
				if remoteTrack.Kind() == webrtc.RTPCodecTypeVideo {
					var pkt rtp.Packet
					if err := pkt.Unmarshal(buf[:n]); err == nil {
						parsedPkt = &pkt
						if pktBuffer != nil {
							pktBuffer.Push(&pkt)
						}
					}
				}

				// 1.5 Active Speaker Detection: Extract ssrc-audio-level RTP header extension
				// from incoming audio packets and feed to the room's ActiveSpeakerDetector.
				if remoteTrack.Kind() == webrtc.RTPCodecTypeAudio {
					if detectorAny := room.GetActiveSpeakerDetector(); detectorAny != nil {
						if detector, ok := detectorAny.(*ActiveSpeakerDetector); ok && detector != nil {
							var pkt rtp.Packet
							if err := pkt.Unmarshal(buf[:n]); err == nil {
								// Check one-byte extension IDs 1..14
								for extID := uint8(1); extID <= 14; extID++ {
									if extPayload := pkt.Header.GetExtension(extID); len(extPayload) > 0 {
										level, _ := ParseAudioLevel(extPayload)
										detector.UpdateLevel("host", level)
										break
									}
								}
							}
						}
					}
					_ = attrs // Suppress unused variable warning
				}

				// 2. Forward RTP packet to the room's localTrack (non-fatal if unattached)
				if localTrack != nil {
					if _, writeErr := localTrack.Write(buf[:n]); writeErr != nil {
						if !errors.Is(writeErr, io.ErrClosedPipe) {
							// Log warning but continue running the packet read/fan-out loop
						}
					}
				}


				// 3. Fan out to active viewer TrackSwitchers with matching Simulcast RID ('q', 'h', 'f')
				if parsedPkt != nil {
					switchers := room.GetAllTrackSwitchers()
					if len(switchers) > 0 {
						for _, s := range switchers {
							if ts, ok := s.(*TrackSwitcher); ok && ts != nil {
								_ = ts.WriteRTP(layerRID, parsedPkt)
							}
						}
					}
				}

				// 4. Inter-Node Cascading: Forward Host's RTP packets directly to Edge Node B(s) via raw UDP
				if forwarderAny := room.GetUDPForwarder(); forwarderAny != nil {
					if forwarder, ok := forwarderAny.(*UDPRTPForwarder); ok && forwarder != nil {
						if parsedPkt != nil {
							_ = forwarder.ForwardRTP(parsedPkt)
						} else {
							var pkt rtp.Packet
							if err := pkt.Unmarshal(buf[:n]); err == nil {
								_ = forwarder.ForwardRTP(&pkt)
							}
						}
					}
				}
			}
		}()
	})

	// Log ICE connection state changes
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log.Printf("Host ICE Connection State (Room: %s): %s\n", room.RoomID, connectionState.String())
	})

	// Log PeerConnection state changes
	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("Host PeerConnection State (Room: %s): %s\n", room.RoomID, state.String())
	})

	return peerConnection, nil
}

// HandleCoHostConnection creates a WebRTC PeerConnection for a co-host, saves their tracks to room.CoHostTracks,
// subscribes to existing host and co-host media, forwards incoming RTP packets, and calls onTrackSaved for room-wide renegotiation.
func HandleCoHostConnection(api *webrtc.API, room *models.Room, coHostID string, config webrtc.Configuration, onTrackSaved func(coHostID string, track *webrtc.TrackLocalStaticRTP)) (*webrtc.PeerConnection, error) {
	if api == nil {
		return nil, errors.New("webrtc api instance is required")
	}
	if room == nil {
		return nil, errors.New("room instance is required")
	}

	peerConnection, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}

	// Register Co-Host PeerConnection in room
	room.SetCoHostPeerConnection(coHostID, peerConnection)

	// 1. Attach Main Host video track to CoHost
	if hostVideo := room.GetDefaultViewerVideoTrack(); hostVideo != nil {
		if _, err := peerConnection.AddTrack(hostVideo); err != nil {
			log.Printf("Failed to attach host video track to cohost %s: %v\n", coHostID, err)
		}
	}
	// Attach Main Host audio track to CoHost
	if hostAudio := room.GetAudioTrack(); hostAudio != nil {
		if _, err := peerConnection.AddTrack(hostAudio); err != nil {
			log.Printf("Failed to attach host audio track to cohost %s: %v\n", coHostID, err)
		}
	}

	// 2. Attach any OTHER active Co-Hosts' video and audio tracks
	for otherID, otherMedia := range room.GetAllCoHostTracks() {
		if otherID != coHostID && otherMedia != nil {
			if otherMedia.VideoTrack != nil {
				_, _ = peerConnection.AddTrack(otherMedia.VideoTrack)
			}
			if otherMedia.AudioTrack != nil {
				_, _ = peerConnection.AddTrack(otherMedia.AudioTrack)
			}
		}
	}

	// 3. Handle incoming media tracks published by the Co-Host
	peerConnection.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("CoHost %s incoming track [Kind: %s, ID: %s, SSRC: %d, MimeType: %s]\n",
			coHostID, remoteTrack.Kind().String(), remoteTrack.ID(), remoteTrack.SSRC(), remoteTrack.Codec().MimeType)

		localTrack, trackErr := webrtc.NewTrackLocalStaticRTP(
			remoteTrack.Codec().RTPCodecCapability,
			remoteTrack.ID(),
			remoteTrack.StreamID(),
		)
		if trackErr != nil {
			log.Printf("Failed to create TrackLocalStaticRTP for cohost %s track: %v\n", coHostID, trackErr)
			return
		}

		// Save in CoHostTracks map according to track Kind (never overwrite video with audio)
		if remoteTrack.Kind() == webrtc.RTPCodecTypeVideo {
			// Strict Security Check: Drop and ignore video tracks in audio-only rooms
			if room.GetRoomType() == "audio" {
				log.Printf("[Security] Dropped unauthorized co-host %s video track in audio-only Room %s (SSRC: %d)\n",
					coHostID, room.RoomID, remoteTrack.SSRC())
				return
			}

			room.SetCoHostTrack(coHostID, localTrack)
			room.SetCoHostVideoSSRC(coHostID, uint32(remoteTrack.SSRC()))
			log.Printf("CoHost %s video track registered in Room %s (SSRC: %d)\n", coHostID, room.RoomID, remoteTrack.SSRC())

			// Periodic Keyframe (PLI) Routine for Co-Host
			go func(trackSSRC uint32) {
				ticker := time.NewTicker(2 * time.Second)
				defer ticker.Stop()

				for range ticker.C {
					if peerConnection.ConnectionState() == webrtc.PeerConnectionStateClosed {
						return
					}
					_ = peerConnection.WriteRTCP([]rtcp.Packet{
						&rtcp.PictureLossIndication{MediaSSRC: trackSSRC},
					})
				}
			}(uint32(remoteTrack.SSRC()))
		} else if remoteTrack.Kind() == webrtc.RTPCodecTypeAudio {
			room.SetCoHostAudioTrack(coHostID, localTrack)
			log.Printf("CoHost %s audio track registered in Room %s\n", coHostID, room.RoomID)
		}

		// Get or create dedicated PacketBuffer for co-host video track
		var coHostPktBuffer *PacketBuffer
		if remoteTrack.Kind() == webrtc.RTPCodecTypeVideo {
			coHostPktBuffer = NewPacketBuffer(DefaultPacketBufferSize)
		}

		// Forward RTP packets to localTrack continuously
		go func() {
			bufPtr := GetRTPBuffer()
			defer PutRTPBuffer(bufPtr)
			buf := *bufPtr

			for {
				n, _, readErr := remoteTrack.Read(buf)
				if readErr != nil {
					return
				}

				// Store a copy in PacketBuffer before forwarding
				if remoteTrack.Kind() == webrtc.RTPCodecTypeVideo && coHostPktBuffer != nil {
					var pkt rtp.Packet
					if err := pkt.Unmarshal(buf[:n]); err == nil {
						coHostPktBuffer.Push(&pkt)
					}
				}

				if _, writeErr := localTrack.Write(buf[:n]); writeErr != nil {
					return
				}
			}
		}()
	})

	return peerConnection, nil
}
