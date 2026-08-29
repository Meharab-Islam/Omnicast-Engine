package webrtc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pion/rtp"
	"omnicast/internal/models"
)

// DefaultUDPCascadePort is the standard UDP port for inter-node RTP forwarding
const DefaultUDPCascadePort = 5004

// UDPRTPForwarder manages direct raw UDP RTP packet forwarding from Origin Node A to Edge Node B(s)
type UDPRTPForwarder struct {
	conn       *net.UDPConn
	destNodes  map[string]*net.UDPAddr // edgeNodeID -> destination UDPAddr
	mu         sync.RWMutex
	closed     bool
	bufferPool *sync.Pool
}

// NewUDPRTPForwarder initializes a new UDPRTPForwarder instance on Origin Node A
func NewUDPRTPForwarder() (*UDPRTPForwarder, error) {
	// Listen on an ephemeral local UDP port for sending
	lAddr, err := net.ResolveUDPAddr("udp", ":0")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve local UDP address: %w", err)
	}

	conn, err := net.ListenUDP("udp", lAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to bind UDP forwarder socket: %w", err)
	}

	return &UDPRTPForwarder{
		conn:      conn,
		destNodes: make(map[string]*net.UDPAddr),
		bufferPool: &sync.Pool{
			New: func() any {
				b := make([]byte, 1500)
				return &b
			},
		},
	}, nil
}

// AddEdgeNode registers a destination Edge Node B IP and port for RTP cascading
func (f *UDPRTPForwarder) AddEdgeNode(edgeNodeID, edgeIP string, port int) error {
	if port <= 0 {
		port = DefaultUDPCascadePort
	}

	targetAddr := fmt.Sprintf("%s:%d", edgeIP, port)
	udpAddr, err := net.ResolveUDPAddr("udp", targetAddr)
	if err != nil {
		return fmt.Errorf("failed to resolve edge node UDP address (%s): %w", targetAddr, err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.destNodes[edgeNodeID] = udpAddr
	log.Printf("[Inter-Node UDP Cascade] Node A added forwarding destination Edge Node '%s' at %s\n",
		edgeNodeID, targetAddr)
	return nil
}

// RemoveEdgeNode removes an Edge Node from the forwarder destination list
func (f *UDPRTPForwarder) RemoveEdgeNode(edgeNodeID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.destNodes, edgeNodeID)
	log.Printf("[Inter-Node UDP Cascade] Node A removed forwarding destination Edge Node '%s'\n", edgeNodeID)
}

// ForwardRTP serializes the Host's RTP packet and writes it directly over raw UDP to all registered Edge Nodes
func (f *UDPRTPForwarder) ForwardRTP(packet *rtp.Packet) error {
	if packet == nil {
		return errors.New("packet is nil")
	}

	f.mu.RLock()
	if f.closed || len(f.destNodes) == 0 {
		f.mu.RUnlock()
		return nil
	}

	// Copy active destination addresses to avoid holding read lock during network I/O
	addrs := make([]*net.UDPAddr, 0, len(f.destNodes))
	for _, addr := range f.destNodes {
		addrs = append(addrs, addr)
	}
	f.mu.RUnlock()

	rawBytes, err := packet.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal RTP packet for UDP cascade: %w", err)
	}

	for _, addr := range addrs {
		if _, writeErr := f.conn.WriteToUDP(rawBytes, addr); writeErr != nil {
			log.Printf("[Inter-Node UDP Cascade Error] Failed to write RTP packet to %s: %v\n", addr.String(), writeErr)
		}
	}

	return nil
}

// Close gracefully closes the forwarder UDP connection
func (f *UDPRTPForwarder) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return nil
	}
	f.closed = true
	if f.conn != nil {
		return f.conn.Close()
	}
	return nil
}

// UDPRTPReceiver runs on Edge Node B, listening on a UDP socket for incoming cascaded RTP packets from Node A
type UDPRTPReceiver struct {
	conn       *net.UDPConn
	listenAddr *net.UDPAddr
	room       *models.Room
	ctx        context.Context
	cancel     context.CancelFunc
	closed     bool
	mu         sync.Mutex
	onPacket   func(packet *rtp.Packet)
}

// NewUDPRTPReceiver creates and binds a UDP listener on Edge Node B
func NewUDPRTPReceiver(listenPort int, room *models.Room) (*UDPRTPReceiver, error) {
	if listenPort <= 0 {
		listenPort = DefaultUDPCascadePort
	}

	lAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", listenPort))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve UDP receiver address: %w", err)
	}

	conn, err := net.ListenUDP("udp", lAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on UDP receiver port %d: %w", listenPort, err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	receiver := &UDPRTPReceiver{
		conn:       conn,
		listenAddr: lAddr,
		room:       room,
		ctx:        ctx,
		cancel:     cancel,
	}

	go receiver.listenLoop()
	log.Printf("[Inter-Node UDP Cascade] Edge Node B listening for cascaded RTP streams on %s\n", lAddr.String())
	return receiver, nil
}

// SetPacketHandler sets a custom handler for incoming RTP packets (e.g. for testing or track writing)
func (r *UDPRTPReceiver) SetPacketHandler(handler func(packet *rtp.Packet)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onPacket = handler
}

// listenLoop continuously reads raw UDP packets from Node A and injects them into Edge Node B's local room tracks
func (r *UDPRTPReceiver) listenLoop() {
	buf := make([]byte, 1500)

	for {
		select {
		case <-r.ctx.Done():
			return
		default:
		}

		_ = r.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, _, err := r.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			r.mu.Lock()
			isClosed := r.closed
			r.mu.Unlock()
			if isClosed {
				return
			}
			continue
		}

		if n <= 0 {
			continue
		}

		packet := &rtp.Packet{}
		if unmarshalErr := packet.Unmarshal(buf[:n]); unmarshalErr != nil {
			continue
		}

		r.mu.Lock()
		handler := r.onPacket
		r.mu.Unlock()

		if handler != nil {
			handler(packet)
		} else if r.room != nil {
			// Edge Node B Local Fan-Out: Forward single received RTP packet to all local viewers,
			// shielding Origin Node A from massive fan-out bandwidth.
			if packet.Header.PayloadType == 96 || packet.Header.PayloadType == 100 || packet.Header.PayloadType == 102 {
				// 1. Write to local room video track
				if r.room.VideoTrack != nil {
					_ = r.room.VideoTrack.WriteRTP(packet)
				}

				// 2. Fan out to active local viewer TrackSwitchers
				switchers := r.room.GetAllTrackSwitchers()
				for _, s := range switchers {
					if ts, ok := s.(*TrackSwitcher); ok && ts != nil {
						_ = ts.WriteRTP(ts.GetCurrentLayer(), packet)
					}
				}

				// 3. Dispatch through FanOutDispatcher worker pool if enabled
				if foAny := r.room.GetFanOutDispatcher(); foAny != nil {
					if fo, ok := foAny.(*FanOutDispatcher); ok && fo != nil {
						_ = fo.Broadcast(packet, nil, nil)
					}
				}
			} else if packet.Header.PayloadType == 111 || packet.Header.PayloadType == 63 {
				// 1. Write to local room audio track (relayed to all local listeners)
				if r.room.AudioTrack != nil {
					_ = r.room.AudioTrack.WriteRTP(packet)
				}
			}
		}
	}
}

// Close closes the UDP listener socket and stops the receive loop
func (r *UDPRTPReceiver) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}
	r.closed = true
	r.cancel()
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}
