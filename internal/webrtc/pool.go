package webrtc

import (
	"sync"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// RTPBufferSize is standard MTU packet size for RTP/RTCP packets (1500 bytes)
const RTPBufferSize = 1500

var rtpBufferPool = sync.Pool{
	New: func() any {
		b := make([]byte, RTPBufferSize)
		return &b
	},
}

// GetRTPBuffer retrieves a 1500-byte slice pointer from the pool (zero-allocation)
func GetRTPBuffer() *[]byte {
	return rtpBufferPool.Get().(*[]byte)
}

// PutRTPBuffer returns a 1500-byte slice pointer to the pool for reuse
func PutRTPBuffer(b *[]byte) {
	if b != nil {
		rtpBufferPool.Put(b)
	}
}

// rtpPacketPool provides reusable rtp.Packet structs across the forwarding pipeline
var rtpPacketPool = sync.Pool{
	New: func() any {
		return &rtp.Packet{}
	},
}

// GetRTPPacket retrieves a recycled rtp.Packet instance from the pool
func GetRTPPacket() *rtp.Packet {
	p := rtpPacketPool.Get().(*rtp.Packet)
	*p = rtp.Packet{}
	return p
}

// PutRTPPacket returns an rtp.Packet back to the pool after reset
func PutRTPPacket(p *rtp.Packet) {
	if p != nil {
		*p = rtp.Packet{}
		rtpPacketPool.Put(p)
	}
}

// pliPacketPool provides reusable rtcp.PictureLossIndication packets
var pliPacketPool = sync.Pool{
	New: func() any {
		return &rtcp.PictureLossIndication{}
	},
}

// GetPLIPacket retrieves a PictureLossIndication from the pool
func GetPLIPacket(mediaSSRC uint32) *rtcp.PictureLossIndication {
	p := pliPacketPool.Get().(*rtcp.PictureLossIndication)
	p.MediaSSRC = mediaSSRC
	p.SenderSSRC = 0
	return p
}

// PutPLIPacket returns a PictureLossIndication back to the pool
func PutPLIPacket(p *rtcp.PictureLossIndication) {
	if p != nil {
		p.MediaSSRC = 0
		p.SenderSSRC = 0
		pliPacketPool.Put(p)
	}
}
