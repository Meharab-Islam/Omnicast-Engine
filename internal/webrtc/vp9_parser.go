package webrtc

import (
	"errors"
)

// VP9PayloadDescriptor represents the parsed fields of a VP9 RTP Payload Descriptor
// as specified in IETF draft-ietf-payload-vp9.
type VP9PayloadDescriptor struct {
	// First byte flags
	HasPictureID    bool // I bit: Picture ID is present
	IsInterPicture  bool // P bit: Inter-picture predicted frame (false = Intra / Keyframe)
	HasLayerIndices bool // L bit: Layer indices (TID, SID, TL0PICIDX) are present
	IsFlexibleMode  bool // F bit: Flexible mode (1) or non-flexible mode (0)
	StartOfFrame    bool // B bit: Start of a VP9 frame
	EndOfFrame      bool // E bit: End of a VP9 frame
	HasSS           bool // V bit: Scalability Structure (SS) is present
	NonReference    bool // Z bit: Not a reference frame for other frames

	// Picture ID (7-bit or 15-bit)
	PictureID uint16

	// Layer indices (present when HasLayerIndices == true)
	// S: Spatial Layer ID (0..7)
	// T: Temporal Layer ID (0..7)
	S           uint8 // S: Spatial Layer ID (SID, 0..7)
	T           uint8 // T: Temporal Layer ID (TID, 0..7)
	SpatialID   uint8 // SpatialID alias for S
	TemporalID  uint8 // TemporalID alias for T
	SwitchingUp bool  // U bit: Switching point (up-switch allowed)
	InterLayer  bool  // D bit: Inter-layer dependency (depends on lower spatial layer)
	TL0PICIDX   uint8 // TL0PICIDX: Temporal layer 0 picture index (non-flexible mode)

	// Scalability Structure (SS) metadata when HasSS == true
	NumSpatialLayers uint8

	// PayloadOffset indicates the byte index where the uncompressed VP9 bitstream starts
	PayloadOffset int
}

// IsKeyframe returns true if the packet contains the start of a VP9 Keyframe (Intra-frame).
func (d *VP9PayloadDescriptor) IsKeyframe() bool {
	return !d.IsInterPicture && d.StartOfFrame
}

// VP9PayloadParser parses VP9 RTP payload descriptors from incoming RTP packets.
type VP9PayloadParser struct{}

// NewVP9PayloadParser creates a new instance of VP9PayloadParser.
func NewVP9PayloadParser() *VP9PayloadParser {
	return &VP9PayloadParser{}
}

// Parse extracts the VP9PayloadDescriptor from a raw VP9 RTP payload byte slice.
func (p *VP9PayloadParser) Parse(payload []byte) (*VP9PayloadDescriptor, error) {
	return ParseVP9Descriptor(payload)
}

// ExtractLayers extracts the Spatial Layer ID (S) and Temporal Layer ID (T) bits directly from the VP9 payload.
// Returns (s uint8, t uint8, hasLayers bool, err error).
func (p *VP9PayloadParser) ExtractLayers(payload []byte) (s uint8, t uint8, hasLayers bool, err error) {
	desc, err := ParseVP9Descriptor(payload)
	if err != nil {
		return 0, 0, false, err
	}
	if !desc.HasLayerIndices {
		return 0, 0, false, nil
	}
	return desc.S, desc.T, true, nil
}

// ParseVP9Descriptor parses the raw VP9 RTP payload descriptor header.
func ParseVP9Descriptor(payload []byte) (*VP9PayloadDescriptor, error) {
	if len(payload) == 0 {
		return nil, errors.New("empty payload")
	}

	offset := 0

	// 1. Mandatory First Byte: [I|P|L|F|B|E|V|Z]
	firstByte := payload[offset]
	offset++

	desc := &VP9PayloadDescriptor{
		HasPictureID:    (firstByte & 0x80) != 0,
		IsInterPicture:  (firstByte & 0x40) != 0,
		HasLayerIndices: (firstByte & 0x20) != 0,
		IsFlexibleMode:  (firstByte & 0x10) != 0,
		StartOfFrame:    (firstByte & 0x08) != 0,
		EndOfFrame:      (firstByte & 0x04) != 0,
		HasSS:           (firstByte & 0x02) != 0,
		NonReference:    (firstByte & 0x01) != 0,
	}

	// 2. Optional Picture ID (I bit)
	if desc.HasPictureID {
		if offset >= len(payload) {
			return nil, errors.New("truncated VP9 payload: missing PictureID")
		}
		pidByte := payload[offset]
		offset++

		is15Bit := (pidByte & 0x80) != 0
		if is15Bit {
			if offset >= len(payload) {
				return nil, errors.New("truncated VP9 payload: missing second byte of 15-bit PictureID")
			}
			desc.PictureID = (uint16(pidByte&0x7F) << 8) | uint16(payload[offset])
			offset++
		} else {
			desc.PictureID = uint16(pidByte & 0x7F)
		}
	}

	// 3. Optional Layer Indices (L bit) - Extracting S (Spatial) and T (Temporal) bits:
	// Layer Byte: [ TID (3 bits: 0..2) | U (1 bit: 3) | SID (3 bits: 4..6) | D (1 bit: 7) ]
	if desc.HasLayerIndices {
		if offset >= len(payload) {
			return nil, errors.New("truncated VP9 payload: missing Layer Indices")
		}
		layerByte := payload[offset]
		offset++

		// Extract T (Temporal Layer ID) from bits 0..2: (layerByte >> 5) & 0x07
		desc.T = (layerByte >> 5) & 0x07
		desc.TemporalID = desc.T

		// Extract U (Switching point) from bit 3: (layerByte & 0x10) != 0
		desc.SwitchingUp = (layerByte & 0x10) != 0

		// Extract S (Spatial Layer ID) from bits 4..6: (layerByte >> 1) & 0x07
		desc.S = (layerByte >> 1) & 0x07
		desc.SpatialID = desc.S

		// Extract D (Inter-layer dependency) from bit 7: (layerByte & 0x01) != 0
		desc.InterLayer = (layerByte & 0x01) != 0

		// In non-flexible mode (F == 0), TL0PICIDX is present
		if !desc.IsFlexibleMode {
			if offset >= len(payload) {
				return nil, errors.New("truncated VP9 payload: missing TL0PICIDX")
			}
			desc.TL0PICIDX = payload[offset]
			offset++
		}
	}

	// 4. Optional Reference Indices in Flexible Mode (F == 1 and P == 1)
	if desc.IsFlexibleMode && desc.IsInterPicture {
		for {
			if offset >= len(payload) {
				return nil, errors.New("truncated VP9 payload: missing P_DIFF reference index")
			}
			pDiffByte := payload[offset]
			offset++
			// Bit 0 (N) indicates if another P_DIFF byte follows
			hasMore := (pDiffByte & 0x01) != 0
			if !hasMore {
				break
			}
		}
	}

	// 5. Optional Scalability Structure (SS) (V bit)
	if desc.HasSS {
		if offset >= len(payload) {
			return nil, errors.New("truncated VP9 payload: missing Scalability Structure")
		}
		ssByte := payload[offset]
		offset++

		nS := ((ssByte >> 5) & 0x07) + 1 // Number of spatial layers
		desc.NumSpatialLayers = nS
		hasY := (ssByte & 0x10) != 0      // Spatial layer resolution present

		// If Y bit is set, skip width/height (4 bytes per spatial layer)
		if hasY {
			skipBytes := int(nS) * 4
			if offset+skipBytes > len(payload) {
				return nil, errors.New("truncated VP9 payload: missing SS resolution data")
			}
			offset += skipBytes
		}

		// Optional Picture grouping description in SS (G bit)
		hasG := (ssByte & 0x08) != 0
		if hasG {
			if offset >= len(payload) {
				return nil, errors.New("truncated VP9 payload: missing SS picture grouping count")
			}
			nG := int(payload[offset])
			offset++

			for i := 0; i < nG; i++ {
				if offset >= len(payload) {
					return nil, errors.New("truncated VP9 payload: missing PG descriptor")
				}
				pgByte := payload[offset]
				offset++
				rCount := int((pgByte >> 2) & 0x03)
				offset += rCount // skip reference diffs
			}
		}
	}

	desc.PayloadOffset = offset
	return desc, nil
}

// IsVP9Keyframe returns true if the incoming VP9 RTP payload represents a Keyframe (I-frame).
func IsVP9Keyframe(payload []byte) bool {
	desc, err := ParseVP9Descriptor(payload)
	if err != nil || desc == nil {
		return false
	}
	return desc.IsKeyframe()
}
