package webrtc

import (
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
	"live-media-server/internal/models"
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

	// Listen for incoming media tracks from the host
	peerConnection.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		rid := remoteTrack.RID()
		log.Printf("Host track received: Kind=%s, RID=%s\n", remoteTrack.Kind().String(), rid)

		trackID := remoteTrack.ID()
		if rid != "" {
			trackID = fmt.Sprintf("%s-%s", remoteTrack.ID(), rid)
		}

		// Create TrackLocalStaticRTP to broadcast media to room viewers
		localTrack, trackErr := webrtc.NewTrackLocalStaticRTP(
			remoteTrack.Codec().RTPCodecCapability,
			trackID,
			remoteTrack.StreamID(),
		)
		if trackErr != nil {
			log.Printf("Failed to create TrackLocalStaticRTP for host track (RID: '%s'): %v\n", rid, trackErr)
			return
		}

		// Register track into the room
		switch remoteTrack.Kind() {
		case webrtc.RTPCodecTypeVideo:
			if rid == "" {
				// Non-simulcast standard video track
				room.SetVideoTrack(localTrack)
				room.SetVideoTrackSSRC("default", uint32(remoteTrack.SSRC()))
				log.Printf("Default non-simulcast video track registered for Room: %s (SSRC: %d)\n", room.RoomID, remoteTrack.SSRC())
			} else {
				// Simulcast quality layer ('q' = Low, 'h' = Medium, 'f' = High)
				room.SetVideoTrackRID(rid, localTrack)
				room.SetVideoTrackSSRC(rid, uint32(remoteTrack.SSRC()))
				log.Printf("Simulcast video layer (RID: '%s') registered in Room.VideoTracks for Room: %s (SSRC: %d)\n", rid, room.RoomID, remoteTrack.SSRC())
			}

			// Send periodic PLI (Picture Loss Indication) keyframe requests to host
			go func() {
				ticker := time.NewTicker(3 * time.Second)
				defer ticker.Stop()
				for range ticker.C {
					if peerConnection.ConnectionState() == webrtc.PeerConnectionStateClosed {
						return
					}
					_ = peerConnection.WriteRTCP([]rtcp.Packet{
						&rtcp.PictureLossIndication{MediaSSRC: uint32(remoteTrack.SSRC())},
					})
				}
			}()

		case webrtc.RTPCodecTypeAudio:
			room.SetAudioTrack(localTrack)
			log.Printf("Audio track registered for Room: %s\n", room.RoomID)
		}

		// Infinite background goroutine to read RTP packets from host and write to room track
		go func() {
			buf := make([]byte, 1500)
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

				// Forward RTP packet to the room's localTrack
				if _, writeErr := localTrack.Write(buf[:n]); writeErr != nil {
					if !errors.Is(writeErr, io.ErrClosedPipe) {
						log.Printf("Error writing RTP packet to room local track: %v\n", writeErr)
					}
					return
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

// HandleCoHostConnection creates a WebRTC PeerConnection for a co-host, saves their track to room.CoHostTracks,
// forwards incoming RTP packets, and calls onTrackSaved callback so the signaling layer can broadcast new_cohost.
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

	peerConnection.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("CoHost %s incoming track [Kind: %s, ID: %s, SSRC: %d, MimeType: %s]\n",
			coHostID, remoteTrack.Kind().String(), remoteTrack.ID(), remoteTrack.SSRC(), remoteTrack.Codec().MimeType)

		localTrack, trackErr := webrtc.NewTrackLocalStaticRTP(
			remoteTrack.Codec().RTPCodecCapability,
			remoteTrack.ID(),
			remoteTrack.StreamID(),
		)
		if trackErr != nil {
			log.Printf("Failed to create TrackLocalStaticRTP for cohost track: %v\n", trackErr)
			return
		}

		// Save in CoHostTracks map
		room.SetCoHostTrack(coHostID, localTrack)
		log.Printf("CoHost track registered in Room %s for CoHost %s\n", room.RoomID, coHostID)

		if onTrackSaved != nil {
			onTrackSaved(coHostID, localTrack)
		}

		// Send periodic PLI if video
		if remoteTrack.Kind() == webrtc.RTPCodecTypeVideo {
			go func() {
				ticker := time.NewTicker(3 * time.Second)
				defer ticker.Stop()
				for range ticker.C {
					if peerConnection.ConnectionState() == webrtc.PeerConnectionStateClosed {
						return
					}
					_ = peerConnection.WriteRTCP([]rtcp.Packet{
						&rtcp.PictureLossIndication{MediaSSRC: uint32(remoteTrack.SSRC())},
					})
				}
			}()
		}

		// Forward RTP packets to localTrack
		go func() {
			buf := make([]byte, 1500)
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
