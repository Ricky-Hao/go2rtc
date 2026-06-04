package miss

import (
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/h265"
)

type packetClassifier struct {
	hdrChanSeen   [2]bool          // hdr[28] values seen
	flagsChanSeen [2]bool          // (flags >> 24) values seen
	resolutions   map[uint32]uint8 // resolution area -> channel
	lastTS        [2]uint64        // last timestamp per channel
	tsInit        [2]bool          // whether lastTS is initialized
}

func newPacketClassifier() *packetClassifier {
	return &packetClassifier{
		resolutions: make(map[uint32]uint8),
	}
}

// Classify runs through classification strategies in priority order.
// The caller must serialize calls.
func (c *packetClassifier) Classify(pkt *Packet) uint8 {
	ch := c.classify(pkt)
	c.lastTS[ch] = pkt.Timestamp
	c.tsInit[ch] = true
	return ch
}

func (c *packetClassifier) classify(pkt *Packet) uint8 {
	// Strategy 1: hdr[28] channel field.
	// Trusted only after seeing both 0 and 1.
	if pkt.ChannelOK {
		c.hdrChanSeen[pkt.Channel] = true
		if c.hdrChanSeen[0] && c.hdrChanSeen[1] {
			return pkt.Channel
		}
	}

	// Strategy 2: (flags >> 24) & 0x01.
	fch := pkt.FlagsChannel
	c.flagsChanSeen[fch] = true
	if c.flagsChanSeen[0] && c.flagsChanSeen[1] {
		return fch
	}

	// Strategy 3: Resolution from SPS in keyframes.
	if ch, ok := c.classifyByResolution(pkt); ok {
		return ch
	}

	// Strategy 4: Timestamp continuity.
	if c.tsInit[0] && c.tsInit[1] {
		return c.classifyByTimestamp(pkt)
	}

	return 0
}

// classifyByTimestamp routes by closest preceding timestamp.
func (c *packetClassifier) classifyByTimestamp(pkt *Packet) uint8 {
	ts := pkt.Timestamp

	if ts == c.lastTS[0] {
		return 0
	}
	if ts == c.lastTS[1] {
		return 1
	}

	var d0, d1 uint64
	if ts >= c.lastTS[0] {
		d0 = ts - c.lastTS[0]
	} else {
		d0 = ^uint64(0)
	}
	if ts >= c.lastTS[1] {
		d1 = ts - c.lastTS[1]
	} else {
		d1 = ^uint64(0)
	}

	if d0 <= d1 {
		return 0
	}
	return 1
}

// classifyByResolution uses SPS from keyframes to map resolution -> channel.
// Higher resolution = channel 0, lower = channel 1.
func (c *packetClassifier) classifyByResolution(pkt *Packet) (uint8, bool) {
	area := videoResolutionArea(pkt)
	if area == 0 {
		return 0, false
	}

	if ch, ok := c.resolutions[area]; ok {
		return ch, true
	}

	c.assignResolution(area)
	if ch, ok := c.resolutions[area]; ok {
		return ch, true
	}
	return 0, false
}

// assignResolution adds a resolution and assigns channels.
// Higher resolution -> channel 0, lower -> channel 1.
func (c *packetClassifier) assignResolution(newArea uint32) {
	c.resolutions[newArea] = 0

	if len(c.resolutions) < 2 {
		return
	}

	var maxArea uint32
	for area := range c.resolutions {
		if area > maxArea {
			maxArea = area
		}
	}

	for area := range c.resolutions {
		if area == maxArea {
			c.resolutions[area] = 0
		} else {
			c.resolutions[area] = 1
		}
	}
}

// videoResolutionArea extracts width*height from H264/H265 SPS in a keyframe.
func videoResolutionArea(pkt *Packet) uint32 {
	switch pkt.CodecID {
	case codecH264:
		avcc := annexb.EncodeToAVCC(pkt.Payload)
		if h264.NALUType(avcc) == h264.NALUTypeSPS {
			sps := h264.DecodeSPS(avcc[4:]) // skip 4-byte AVCC length prefix
			if sps != nil {
				return uint32(sps.Width()) * uint32(sps.Height())
			}
		}
	case codecH265:
		avcc := annexb.EncodeToAVCC(pkt.Payload)
		if h265.NALUType(avcc) == h265.NALUTypeVPS {
			// H265 keyframes start with VPS, then SPS. Find the SPS.
			spsData := findH265SPS(avcc)
			if spsData != nil {
				sps := h265.DecodeSPS(spsData)
				if sps != nil {
					return uint32(sps.Width()) * uint32(sps.Height())
				}
			}
		}
	}
	return 0
}

// findH265SPS searches for an SPS NALU in AVCC-formatted H265 data.
func findH265SPS(avcc []byte) []byte {
	for i := 0; i+4 < len(avcc); {
		size := int(avcc[i])<<24 | int(avcc[i+1])<<16 | int(avcc[i+2])<<8 | int(avcc[i+3])
		i += 4
		if size <= 0 || i+size > len(avcc) {
			break
		}
		naluType := (avcc[i] >> 1) & 0x3F
		if naluType == h265.NALUTypeSPS {
			return avcc[i : i+size]
		}
		i += size
	}
	return nil
}
