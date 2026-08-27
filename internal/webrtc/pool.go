package webrtc

import "sync"

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
