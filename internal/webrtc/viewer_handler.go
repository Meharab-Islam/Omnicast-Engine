package webrtc

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"time"

	"omnicast/internal/models"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
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

		// Read incoming RTCP feedback from viewer (PLI/FIR/NACK/REMB) using dedicated ReadRTCP goroutine
		abr := NewABRController()
		ReadRTCP(videoSender, room, viewerID, switcher, abr)
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
		ReadRTCP(audioSender, room, viewerID, nil, nil)
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

		room.SetCoHostPeerConnection(viewerID, peerConnection)

		if remoteTrack.Kind() == webrtc.RTPCodecTypeVideo {
			// Strict Security Check: Drop and ignore video tracks in audio-only rooms
			if room.GetRoomType() == "audio" {
				log.Printf("[Security] Dropped unauthorized co-host %s video track in audio-only Room %s (SSRC: %d)\n",
					viewerID, room.RoomID, remoteTrack.SSRC())
				return
			}

			room.SetCoHostTrack(viewerID, localTrack)
			room.SetCoHostVideoSSRC(viewerID, uint32(remoteTrack.SSRC()))
			log.Printf("Viewer/CoHost %s video track registered in Room %s (SSRC: %d)\n", viewerID, room.RoomID, remoteTrack.SSRC())
		} else if remoteTrack.Kind() == webrtc.RTPCodecTypeAudio {
			room.SetCoHostAudioTrack(viewerID, localTrack)
			log.Printf("Viewer/CoHost %s audio track registered in Room %s\n", viewerID, room.RoomID)
		}

		// Get or create dedicated PacketBuffer for upgraded co-host video track
		var coHostPktBuffer *PacketBuffer
		if remoteTrack.Kind() == webrtc.RTPCodecTypeVideo {
			coHostPktBuffer = NewPacketBuffer(DefaultPacketBufferSize)
		}

		// Forward incoming RTP continuously using pooled buffer
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

// ReadRTCP starts a dedicated background goroutine that constantly reads and processes incoming RTCP packets
// (e.g. PLI, FIR, NACK, REMB) from the viewer's RTPSender / RTCPeerConnection.
func ReadRTCP(sender *webrtc.RTPSender, room *models.Room, viewerID string, switcher *TrackSwitcher, abr *ABRController) {
	if sender == nil {
		return
	}

	go func() {
		rtcpBuf := make([]byte, 1500)

		for {
			n, _, rtcpErr := sender.Read(rtcpBuf)
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
						if abr != nil && switcher != nil {
							estimatedBps := uint64(p.Bitrate)
							optimalLayer := abr.EvaluateLayer(estimatedBps, 0.0)
							if switcher.GetCurrentLayer() != optimalLayer {
								log.Printf("[ABR Auto-Switch] Room %s: Viewer %s switching layer %s -> %s (Bitrate: %d bps)\n",
									room.RoomID, viewerID, switcher.GetCurrentLayer(), optimalLayer, estimatedBps)
								switcher.SwitchLayer(optimalLayer)
								if room != nil {
									room.SendPLIThrottled(1 * time.Second)
								}
							}
						}
					case *rtcp.TransportLayerNack:
						// Loop through NackPair list to extract all lost sequence numbers requested by the Viewer
						lostSeqs := ExtractLostSequenceNumbers(p.Nacks)

						log.Printf("[NACK Received] Room %s: Viewer %s requested retransmission for %d lost RTP packet(s) on MediaSSRC %d (Seqs: %v)\n",
							room.RoomID, viewerID, len(lostSeqs), p.MediaSSRC, lostSeqs)

						// For each lost sequence number, retrieve from PacketBuffer and write directly back to Viewer's track
						if room != nil {
							layerKey := "default"
							if switcher != nil {
								layerKey = switcher.GetCurrentLayer()
							}

							var pb *PacketBuffer
							if pbAny := room.GetPacketBuffer(layerKey); pbAny != nil {
								if typedPB, ok := pbAny.(*PacketBuffer); ok && typedPB != nil {
									pb = typedPB
								}
							}
							if pb == nil {
								for _, pbAny := range room.GetAllPacketBuffers() {
									if typedPB, ok := pbAny.(*PacketBuffer); ok && typedPB != nil {
										pb = typedPB
										break
									}
								}
							}

							var missingSeqs []uint16
							if pb != nil {
								for _, seq := range lostSeqs {
									if pkt := pb.Get(seq); pkt != nil {
										if switcher != nil && switcher.outTrack != nil {
											_ = switcher.outTrack.WriteRTP(pkt)
										}
									} else {
										// Packet is too old or not found in buffer -> aggregate for Host NACK
										missingSeqs = append(missingSeqs, seq)
									}
								}
							} else {
								missingSeqs = lostSeqs
							}

							// If any packets were not found in SFU PacketBuffer, aggregate and forward a new NACK to Host
							if len(missingSeqs) > 0 && room.HostPC != nil && room.HostPC.ConnectionState() != webrtc.PeerConnectionStateClosed {
								nackPairs := BuildNackPairs(missingSeqs)
								if len(nackPairs) > 0 {
									hostNack := &rtcp.TransportLayerNack{
										SenderSSRC: p.SenderSSRC,
										MediaSSRC:  p.MediaSSRC,
										Nacks:      nackPairs,
									}
									_ = room.HostPC.WriteRTCP([]rtcp.Packet{hostNack})
									log.Printf("[NACK Forwarded to Host] Room %s: Forwarded NACK for %d unrecovered packet(s) to Host (MediaSSRC %d, Seqs: %v)\n",
										room.RoomID, len(missingSeqs), p.MediaSSRC, missingSeqs)
								}
							}
						}

						if switcher != nil && switcher.GetCurrentLayer() == LayerHigh {
							log.Printf("[ABR Packet Loss] Room %s: Viewer %s NACK detected, downgrading layer '%s' -> '%s'\n",
								room.RoomID, viewerID, LayerHigh, LayerMedium)
							switcher.SwitchLayer(LayerMedium)
							if room != nil {
								room.SendPLIThrottled(1 * time.Second)
							}
						}
					case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
						// Smart PLI throttling: max 1 PLI per second to protect publisher encoder
						if room != nil {
							room.SendPLIThrottled(1 * time.Second)
						}
					}
				}
			} else if room != nil {
				room.SendPLIThrottled(1 * time.Second)
			}
		}
	}()
}

// ExtractLostSequenceNumbers iterates through a slice of rtcp.NackPair and extracts all requested lost RTP sequence numbers.
// It parses the base PacketID and any additional lost packets encoded in the 16-bit LostPackets bitmask.
func ExtractLostSequenceNumbers(nackPairs []rtcp.NackPair) []uint16 {
	var lostSeqs []uint16
	for _, pair := range nackPairs {
		// 1. Append the primary lost sequence number
		lostSeqs = append(lostSeqs, pair.PacketID)

		// 2. Extract subsequent lost sequence numbers from the 16-bit bitmap
		mask := uint16(pair.LostPackets)
		for i := uint16(0); i < 16; i++ {
			if (mask & (1 << i)) != 0 {
				lostSeqs = append(lostSeqs, pair.PacketID+i+1)
			}
		}
	}
	return lostSeqs
}

// BuildNackPairs aggregates a slice of missing sequence numbers into compressed rtcp.NackPair elements with 16-bit bitmasks.
func BuildNackPairs(seqs []uint16) []rtcp.NackPair {
	if len(seqs) == 0 {
		return nil
	}

	sortedSeqs := make([]uint16, len(seqs))
	copy(sortedSeqs, seqs)
	sort.Slice(sortedSeqs, func(i, j int) bool {
		return sortedSeqs[i] < sortedSeqs[j]
	})

	var pairs []rtcp.NackPair
	i := 0
	for i < len(sortedSeqs) {
		baseSeq := sortedSeqs[i]
		var bitmap uint16
		j := i + 1
		for j < len(sortedSeqs) {
			diff := int(sortedSeqs[j] - baseSeq)
			if diff > 0 && diff <= 16 {
				bitmap |= (1 << (diff - 1))
				j++
			} else {
				break
			}
		}
		pairs = append(pairs, rtcp.NackPair{
			PacketID:    baseSeq,
			LostPackets: rtcp.PacketBitmap(bitmap),
		})
		i = j
	}
	return pairs
}
