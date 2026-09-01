package webrtc

import (
	"testing"

	"github.com/pion/rtp"
)

func TestRTPPacketPool(t *testing.T) {
	// 1. Test Buffer Pool
	buf := GetRTPBuffer()
	if buf == nil || len(*buf) != RTPBufferSize {
		t.Fatalf("Expected %d-byte buffer from pool", RTPBufferSize)
	}
	PutRTPBuffer(buf)

	// 2. Test RTP Packet Pool
	pkt := GetRTPPacket()
	if pkt == nil {
		t.Fatal("Expected non-nil rtp.Packet from pool")
	}
	pkt.Header = rtp.Header{
		SequenceNumber: 1234,
		SSRC:           5678,
	}
	pkt.Payload = []byte{1, 2, 3, 4}

	PutRTPPacket(pkt)

	// Retrieve again and verify reset
	recycled := GetRTPPacket()
	if recycled.Header.SequenceNumber != 0 || recycled.Header.SSRC != 0 || len(recycled.Payload) != 0 {
		t.Fatalf("Expected clean reset of recycled rtp.Packet: %+v", recycled)
	}
	PutRTPPacket(recycled)

	// 3. Test PLI Packet Pool
	pli := GetPLIPacket(99999)
	if pli == nil || pli.MediaSSRC != 99999 {
		t.Fatalf("Expected PLI with MediaSSRC 99999, got: %+v", pli)
	}
	PutPLIPacket(pli)
}
