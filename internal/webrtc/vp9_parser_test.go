package webrtc

import (
	"testing"
)

func TestVP9PayloadParser_Keyframe(t *testing.T) {
	parser := NewVP9PayloadParser()

	// 1. Simple Keyframe: B=1 (Start of frame, 0x08), P=0 (Intra frame)
	payload := []byte{0x08, 0x01, 0x02, 0x03}
	desc, err := parser.Parse(payload)
	if err != nil {
		t.Fatalf("unexpected error parsing simple keyframe: %v", err)
	}

	if !desc.StartOfFrame {
		t.Fatalf("expected StartOfFrame (B=1)")
	}
	if desc.IsInterPicture {
		t.Fatalf("expected Intra-frame (P=0)")
	}
	if !desc.IsKeyframe() {
		t.Fatalf("expected IsKeyframe to be true")
	}
	if !IsVP9Keyframe(payload) {
		t.Fatalf("expected IsVP9Keyframe to be true")
	}
	if desc.PayloadOffset != 1 {
		t.Fatalf("expected payload offset 1, got %d", desc.PayloadOffset)
	}
}

func TestVP9PayloadParser_InterFrame(t *testing.T) {
	parser := NewVP9PayloadParser()

	// Inter-frame: P=1 (0x40), B=1 (0x08) -> 0x48
	payload := []byte{0x48, 0xAA, 0xBB}
	desc, err := parser.Parse(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !desc.IsInterPicture {
		t.Fatalf("expected Inter-picture (P=1)")
	}
	if desc.IsKeyframe() {
		t.Fatalf("expected IsKeyframe to be false for inter-frame")
	}
	if IsVP9Keyframe(payload) {
		t.Fatalf("expected IsVP9Keyframe to be false")
	}
}

func TestVP9PayloadParser_PictureID(t *testing.T) {
	parser := NewVP9PayloadParser()

	// 1. 7-bit Picture ID: I=1 (0x80), B=1 (0x08) -> 0x88
	// PictureID byte: M=0, ID=42 -> 0x2A
	payload7Bit := []byte{0x88, 0x2A, 0x01, 0x02}
	desc7, err := parser.Parse(payload7Bit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !desc7.HasPictureID {
		t.Fatalf("expected HasPictureID = true")
	}
	if desc7.PictureID != 42 {
		t.Fatalf("expected PictureID 42, got %d", desc7.PictureID)
	}
	if desc7.PayloadOffset != 2 {
		t.Fatalf("expected offset 2, got %d", desc7.PayloadOffset)
	}

	// 2. 15-bit Picture ID: I=1, B=1 -> 0x88
	// 15-bit ID: byte1=0x81 (M=1, high=1), byte2=0x20 -> (1<<8) | 0x20 = 288
	payload15Bit := []byte{0x88, 0x81, 0x20, 0x01, 0x02}
	desc15, err := parser.Parse(payload15Bit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if desc15.PictureID != 288 {
		t.Fatalf("expected 15-bit PictureID 288, got %d", desc15.PictureID)
	}
	if desc15.PayloadOffset != 3 {
		t.Fatalf("expected offset 3, got %d", desc15.PayloadOffset)
	}
}

func TestVP9PayloadParser_LayerIndicesSVC(t *testing.T) {
	parser := NewVP9PayloadParser()

	// SVC L3T3 packet:
	// Byte 0: I=1 (0x80), L=1 (0x20), B=1 (0x08) -> 0xA8
	// Byte 1: PictureID (7-bit) = 10 -> 0x0A
	// Byte 2: Layer info: TID=2 (bits 0..2 = 2<<5 = 0x40), U=1 (0x10), SID=1 (bits 4..6 = 1<<1 = 0x02), D=1 (0x01) -> 0x40|0x10|0x02|0x01 = 0x53
	// Byte 3: TL0PICIDX (non-flexible mode F=0) = 15 -> 0x0F
	payload := []byte{0xA8, 0x0A, 0x53, 0x0F, 0xFF, 0xEE}
	desc, err := parser.Parse(payload)
	if err != nil {
		t.Fatalf("unexpected error parsing SVC descriptor: %v", err)
	}

	if !desc.HasLayerIndices {
		t.Fatalf("expected HasLayerIndices = true")
	}
	if desc.T != 2 || desc.TemporalID != 2 {
		t.Fatalf("expected T (Temporal Layer ID) = 2, got T=%d, TID=%d", desc.T, desc.TemporalID)
	}
	if !desc.SwitchingUp {
		t.Fatalf("expected SwitchingUp (U=1)")
	}
	if desc.S != 1 || desc.SpatialID != 1 {
		t.Fatalf("expected S (Spatial Layer ID) = 1, got S=%d, SID=%d", desc.S, desc.SpatialID)
	}
	if !desc.InterLayer {
		t.Fatalf("expected InterLayer (D=1)")
	}
	if desc.TL0PICIDX != 15 {
		t.Fatalf("expected TL0PICIDX 15, got %d", desc.TL0PICIDX)
	}
	if desc.PayloadOffset != 4 {
		t.Fatalf("expected offset 4, got %d", desc.PayloadOffset)
	}

	// Test ExtractLayers helper method
	s, tLayer, hasLayers, extractErr := parser.ExtractLayers(payload)
	if extractErr != nil || !hasLayers {
		t.Fatalf("ExtractLayers failed: %v, hasLayers=%v", extractErr, hasLayers)
	}
	if s != 1 || tLayer != 2 {
		t.Fatalf("ExtractLayers returned wrong values: S=%d (expected 1), T=%d (expected 2)", s, tLayer)
	}
}

func TestVP9PayloadParser_Truncated(t *testing.T) {
	parser := NewVP9PayloadParser()

	// Empty payload
	if _, err := parser.Parse(nil); err == nil {
		t.Fatalf("expected error for nil payload")
	}

	// Truncated PictureID (I=1 but no following byte)
	if _, err := parser.Parse([]byte{0x80}); err == nil {
		t.Fatalf("expected error for truncated PictureID")
	}

	// Truncated Layer indices (L=1 but no following byte)
	if _, err := parser.Parse([]byte{0x20}); err == nil {
		t.Fatalf("expected error for truncated Layer indices")
	}
}
