package webrtc

import (
	"errors"
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	"omnicast/internal/models"
)

// HandleHostConnection creates a WebRTC PeerConnection for the broadcaster/host,
// sets up OnTrack handler, and reads incoming RTP packets in a background goroutine to write to room tracks.
// Supports Simulcast layers ('q' = Low, 'h' = Medium, 'f' = High) via track.RID().
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

	// Maintain a separate ring buffer for every incoming video track from the Host
	packetBuffers := make(map[string]*PacketBuffer)
	var pbMu sync.RWMutex

	// Listen for incoming media tracks from the host
	peerConnection.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		rid := remoteTrack.RID()
		log.Printf("Host track received: Kind=%s, RID=%s\n", remoteTrack.Kind().String(), rid)

		// Get or create dedicated PacketBuffer for this video track layer
		var pktBuffer *PacketBuffer
		if remoteTrack.Kind() == webrtc.RTPCodecTypeVideo {
			layerKey := rid
			if layerKey == "" {
				layerKey = "default"
			}

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
		if rid != "" {
			trackID = fmt.Sprintf("%s-%s", remoteTrack.ID(), rid)
		}

		var localTrack *webrtc.TrackLocalStaticRTP
		var trackErr error

		// Register track into the room
		switch remoteTrack.Kind() {
		case webrtc.RTPCodecTypeVideo:
			// Strict Security Check: Drop and ignore video tracks in audio-only rooms
			if room.GetRoomType() == "audio" {
				log.Printf("[Security] Dropped unauthorized incoming video track %s in audio-only Room %s (SSRC: %d)\n",
					remoteTrack.ID(), room.RoomID, remoteTrack.SSRC())
				return
			}

			if rid == "" {
				// Non-simulcast standard video track
				if existing := room.GetVideoTrack(); existing != nil {
					localTrack = existing
					log.Printf("Re-bound existing default VideoTrack on Host reconnect for Room: %s\n", room.RoomID)
				} else {
					localTrack, trackErr = webrtc.NewTrackLocalStaticRTP(
						remoteTrack.Codec().RTPCodecCapability,
						trackID,
						remoteTrack.StreamID(),
					)
					if trackErr != nil {
						log.Printf("Failed to create TrackLocalStaticRTP for host track: %v\n", trackErr)
						return
					}
					room.SetVideoTrack(localTrack)
					log.Printf("Default non-simulcast video track registered for Room: %s (SSRC: %d)\n", room.RoomID, remoteTrack.SSRC())
				}
				room.SetVideoTrackSSRC("default", uint32(remoteTrack.SSRC()))
				room.SetHostVideoSSRC(uint32(remoteTrack.SSRC()))
			} else {
				// Simulcast quality layer ('q' = Low, 'h' = Medium, 'f' = High)
				if existing := room.GetVideoTrackByRID(rid); existing != nil {
					localTrack = existing
					log.Printf("Re-bound existing Simulcast layer (RID: '%s') on Host reconnect for Room: %s\n", rid, room.RoomID)
				} else {
					localTrack, trackErr = webrtc.NewTrackLocalStaticRTP(
						remoteTrack.Codec().RTPCodecCapability,
						trackID,
						remoteTrack.StreamID(),
					)
					if trackErr != nil {
						log.Printf("Failed to create TrackLocalStaticRTP for host track (RID: '%s'): %v\n", rid, trackErr)
						return
					}
					room.SetVideoTrackRID(rid, localTrack)
					log.Printf("Simulcast video layer (RID: '%s') registered in Room.VideoTracks for Room: %s (SSRC: %d)\n", rid, room.RoomID, remoteTrack.SSRC())
				}
				room.SetVideoTrackSSRC(rid, uint32(remoteTrack.SSRC()))
				if rid == "h" || room.GetHostVideoSSRC() == 0 {
					room.SetHostVideoSSRC(uint32(remoteTrack.SSRC()))
				}
			}

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
				n, _, readErr := remoteTrack.Read(buf)
				if readErr != nil {
					if errors.Is(readErr, io.EOF) {
						log.Printf("Host track %s closed (EOF)\n", remoteTrack.ID())
					} else {
						log.Printf("Error reading RTP packet from host track %s: %v\n", remoteTrack.ID(), readErr)
					}
					return
				}

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

				// 2. Forward RTP packet to the room's localTrack
				if _, writeErr := localTrack.Write(buf[:n]); writeErr != nil {
					if !errors.Is(writeErr, io.ErrClosedPipe) {
						log.Printf("Error writing RTP packet to room local track: %v\n", writeErr)
					}
					return
				}

				// 3. Fan out to active viewer TrackSwitchers for dynamic ABR switching
				if parsedPkt != nil {
					switchers := room.GetAllTrackSwitchers()
					if len(switchers) > 0 {
						effectiveRID := rid
						if effectiveRID == "" {
							effectiveRID = "h"
						}
						for _, s := range switchers {
							if ts, ok := s.(*TrackSwitcher); ok && ts != nil {
								_ = ts.WriteRTP(effectiveRID, parsedPkt)
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
