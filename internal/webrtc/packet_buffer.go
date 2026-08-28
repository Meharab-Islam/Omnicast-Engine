package webrtc

import (
	"sync"

	"github.com/pion/rtp"
)

// DefaultPacketBufferSize defines the default capacity of the packet ring buffer (e.g. 512 packets)
const DefaultPacketBufferSize = 512

// PacketBuffer implements a high-performance circular Ring Buffer for storing and retrieving *rtp.Packet.
// It uses head and tail indices to achieve O(1) packet caching and fast O(1) sequence-number lookups for NACK retransmissions.
type PacketBuffer struct {
	packets  []*rtp.Packet
	capacity int
	size     int
	head     int
	tail     int
	mu       sync.RWMutex
}

// NewPacketBuffer creates and initializes a circular Ring Buffer with the specified capacity
func NewPacketBuffer(capacity int) *PacketBuffer {
	if capacity <= 0 {
		capacity = DefaultPacketBufferSize
	}
	return &PacketBuffer{
		packets:  make([]*rtp.Packet, capacity),
		capacity: capacity,
		size:     0,
		head:     0,
		tail:     0,
	}
}

// Push inserts a cloned *rtp.Packet into the ring buffer.
// If the buffer is full, it safely overwrites the oldest packet at the tail and advances the tail index.
func (pb *PacketBuffer) Push(packet *rtp.Packet) {
	if packet == nil {
		return
	}

	// Deep clone the packet to prevent data races or in-place mutations
	clonedPacket := packet.Clone()

	pb.mu.Lock()
	defer pb.mu.Unlock()

	// If buffer is full, overwrite oldest packet at tail and advance tail
	if pb.size == pb.capacity {
		pb.packets[pb.head] = clonedPacket
		pb.head = (pb.head + 1) % pb.capacity
		pb.tail = (pb.tail + 1) % pb.capacity
	} else {
		pb.packets[pb.head] = clonedPacket
		pb.head = (pb.head + 1) % pb.capacity
		pb.size++
	}
}

// Get retrieves a cached *rtp.Packet by its sequence number.
// Returns the cloned packet if found, or nil if it has been overwritten or not present.
func (pb *PacketBuffer) Get(sequenceNumber uint16) *rtp.Packet {
	pb.mu.RLock()
	defer pb.mu.RUnlock()

	if pb.size == 0 {
		return nil
	}

	// Search backwards from newest packet (head) to oldest (tail)
	for i := 0; i < pb.size; i++ {
		idx := (pb.head - 1 - i + pb.capacity) % pb.capacity
		pkt := pb.packets[idx]
		if pkt != nil && pkt.SequenceNumber == sequenceNumber {
			return pkt.Clone()
		}
	}

	return nil
}

// GetBySequenceNumber retrieves a cached *rtp.Packet by its RTP SequenceNumber.
// Returns (packet, true) if found, or (nil, false) if not present or overwritten.
func (pb *PacketBuffer) GetBySequenceNumber(seq uint16) (*rtp.Packet, bool) {
	pkt := pb.Get(seq)
	if pkt != nil {
		return pkt, true
	}
	return nil, false
}

// Pop removes and returns the oldest *rtp.Packet from the tail of the buffer.
// Returns nil if the buffer is empty.
func (pb *PacketBuffer) Pop() *rtp.Packet {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	if pb.size == 0 {
		return nil
	}

	pkt := pb.packets[pb.tail]
	pb.packets[pb.tail] = nil
	pb.tail = (pb.tail + 1) % pb.capacity
	pb.size--

	return pkt
}

// Size returns the current number of packets in the buffer
func (pb *PacketBuffer) Size() int {
	pb.mu.RLock()
	defer pb.mu.RUnlock()
	return pb.size
}

// Capacity returns the maximum capacity of the ring buffer
func (pb *PacketBuffer) Capacity() int {
	return pb.capacity
}

// Head returns the current head index
func (pb *PacketBuffer) Head() int {
	pb.mu.RLock()
	defer pb.mu.RUnlock()
	return pb.head
}

// Tail returns the current tail index
func (pb *PacketBuffer) Tail() int {
	pb.mu.RLock()
	defer pb.mu.RUnlock()
	return pb.tail
}

// Clear wipes all packets from the buffer and resets head, tail, and size to 0
func (pb *PacketBuffer) Clear() {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	for i := range pb.packets {
		pb.packets[i] = nil
	}
	pb.head = 0
	pb.tail = 0
	pb.size = 0
}
