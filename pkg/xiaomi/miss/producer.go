package miss

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
)

type Producer struct {
	core.Connection
	stream *stream
}

func Dial(rawURL string) (core.Producer, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	query := u.Query()
	channel, err := parseChannel(query)
	if err != nil {
		return nil, err
	}

	sess, st, err := defaultSessionManager.acquire(rawURL, channel)
	if err != nil {
		return nil, err
	}

	audio := query.Get("audio")
	err = sess.startMedia(channel, query.Get("subtype"), audio)
	if err != nil {
		_ = st.Close()
		return nil, err
	}

	medias, err := probe(st, audio != "0")
	if err != nil {
		_ = st.Close()
		return nil, err
	}

	return &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "xiaomi/miss",
			Protocol:   sess.client.Protocol(),
			RemoteAddr: st.RemoteAddr().String(),
			UserAgent:  sess.client.Version(),
			Medias:     medias,
			Transport:  st,
		},
		stream: st,
	}, nil
}

func parseChannel(query url.Values) (uint8, error) {
	raw := query.Get("channel")
	switch raw {
	case "", "0":
		return 0, nil
	case "1":
		return 1, nil
	}
	return 0, fmt.Errorf("xiaomi: unsupported channel: %s", strconv.Quote(raw))
}

func probe(st *stream, audio bool) ([]*core.Media, error) {
	_ = st.SetDeadline(time.Now().Add(15 * time.Second))

	var vcodec, acodec *core.Codec

	for {
		pkt, err := st.ReadPacket()
		if err != nil {
			// If we got video but timed out waiting for audio, that's OK
			// for dual-channel where audio may only go to one stream.
			if vcodec != nil {
				break
			}
			if acodec != nil {
				err = fmt.Errorf("no video")
			}
			return nil, fmt.Errorf("xiaomi: probe: %w", err)
		}

		if audio && acodec == nil {
			acodec = st.session.rememberAudioCodec(audioCodecFromPacket(pkt))
		}

		switch pkt.CodecID {
		case codecH264:
			if vcodec == nil {
				buf := annexb.EncodeToAVCC(pkt.Payload)
				if h264.NALUType(buf) == h264.NALUTypeSPS {
					vcodec = h264.AVCCToCodec(buf)
				}
			}
		case codecH265:
			if vcodec == nil {
				buf := annexb.EncodeToAVCC(pkt.Payload)
				if h265.NALUType(buf) == h265.NALUTypeVPS {
					vcodec = h265.AVCCToCodec(buf)
				}
			}
		}

		if vcodec != nil && (acodec != nil || !audio) {
			break
		}
	}

	_ = st.SetDeadline(time.Time{})

	var talkCodec *core.Codec
	if audio {
		if acodec == nil {
			if acodec = st.session.cachedAudioCodec(); acodec == nil {
				acodec = st.session.defaultAudioCodec()
			}
		}
		talkCodec = st.session.talkCodec()
	}

	medias := []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{vcodec},
		},
	}

	if acodec != nil {
		medias = append(medias, &core.Media{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{acodec},
		})
	}

	if talkCodec != nil {
		medias = append(medias, &core.Media{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs:    []*core.Codec{talkCodec},
		})
	}

	return medias, nil
}

func audioCodecFromPacket(pkt *Packet) *core.Codec {
	switch pkt.CodecID {
	case codecPCM:
		return &core.Codec{Name: core.CodecPCML, ClockRate: pkt.SampleRate()}
	case codecPCMU:
		return &core.Codec{Name: core.CodecPCMU, ClockRate: pkt.SampleRate()}
	case codecPCMA:
		return &core.Codec{Name: core.CodecPCMA, ClockRate: pkt.SampleRate()}
	case codecOPUS:
		return &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}
	}
	return nil
}

const timestamp40ms = 48000 * 0.040

func (p *Producer) Start() error {
	var audioTS uint32

	for {
		pkt, err := p.stream.ReadPacket()
		if err != nil {
			return err
		}

		p.Recv += len(pkt.Payload)

		// TODO: rewrite this
		var name string
		var pkt2 *core.Packet

		switch pkt.CodecID {
		case codecH264, codecH265:
			pkt2 = &core.Packet{
				Header: rtp.Header{
					SequenceNumber: uint16(pkt.Sequence),
					Timestamp:      TimeToRTP(pkt.Timestamp, 90000),
				},
				Payload: annexb.EncodeToAVCC(pkt.Payload),
			}
			if pkt.CodecID == codecH264 {
				name = core.CodecH264
			} else {
				name = core.CodecH265
			}
		case codecPCMA:
			p.stream.session.rememberAudioCodec(audioCodecFromPacket(pkt))
			name = core.CodecPCMA
			pkt2 = &core.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					SequenceNumber: uint16(pkt.Sequence),
					Timestamp:      audioTS,
				},
				Payload: pkt.Payload,
			}
			audioTS += uint32(len(pkt.Payload))
		case codecPCMU:
			p.stream.session.rememberAudioCodec(audioCodecFromPacket(pkt))
			name = core.CodecPCMU
			pkt2 = &core.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					SequenceNumber: uint16(pkt.Sequence),
					Timestamp:      audioTS,
				},
				Payload: pkt.Payload,
			}
			audioTS += uint32(len(pkt.Payload))
		case codecPCM:
			p.stream.session.rememberAudioCodec(audioCodecFromPacket(pkt))
			name = core.CodecPCML
			pkt2 = &core.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					SequenceNumber: uint16(pkt.Sequence),
					Timestamp:      audioTS,
				},
				Payload: pkt.Payload,
			}
			audioTS += uint32(len(pkt.Payload) / 2)
		case codecOPUS:
			p.stream.session.rememberAudioCodec(audioCodecFromPacket(pkt))
			name = core.CodecOpus
			pkt2 = &core.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					SequenceNumber: uint16(pkt.Sequence),
					Timestamp:      audioTS,
				},
				Payload: pkt.Payload,
			}
			// known cameras sends packets with 40ms long
			audioTS += timestamp40ms
		}

		if pkt2 == nil {
			continue
		}

		if p.writePacket(name, pkt2, pkt.SampleRate()) {
			continue
		}
	}
}

func (p *Producer) writePacket(name string, pkt *core.Packet, sampleRate uint32) bool {
	for _, recv := range p.Receivers {
		if matchesPacketCodec(recv.Codec, name, sampleRate) {
			recv.WriteRTP(pkt)
			return true
		}
	}

	if !isPCMCodecName(name) {
		return false
	}

	for _, recv := range p.Receivers {
		if !isPCMCodecName(recv.Codec.Name) {
			continue
		}
		converted := transcodePCMPacket(pkt, name, recv.Codec, sampleRate)
		if converted == nil {
			continue
		}
		recv.WriteRTP(converted)
		return true
	}

	return false
}

func transcodePCMPacket(pkt *core.Packet, srcName string, dst *core.Codec, sampleRate uint32) *core.Packet {
	if sampleRate == 0 {
		sampleRate = 8000
	}

	dstCodec := dst.Clone()
	if dstCodec.ClockRate == 0 {
		dstCodec.ClockRate = sampleRate
	}
	if dstCodec.Name == srcName && dstCodec.ClockRate == sampleRate {
		return pkt
	}

	srcCodec := &core.Codec{Name: srcName, ClockRate: sampleRate}
	payload := pcm.Transcode(dstCodec, srcCodec)(pkt.Payload)

	converted := *pkt
	converted.Payload = payload
	if dstCodec.ClockRate != sampleRate {
		converted.Timestamp = uint32(uint64(pkt.Timestamp) * uint64(dstCodec.ClockRate) / uint64(sampleRate))
	}
	return &converted
}

func matchesPacketCodec(codec *core.Codec, name string, sampleRate uint32) bool {
	if codec.Name != name {
		return false
	}
	if !isPCMCodecName(name) || sampleRate == 0 {
		return true
	}
	return codec.ClockRate == 0 || codec.ClockRate == sampleRate
}

func isPCMCodecName(name string) bool {
	switch name {
	case core.CodecPCMA, core.CodecPCMU, core.CodecPCM, core.CodecPCML:
		return true
	}
	return false
}

func (p *Producer) Stop() error {
	return p.Connection.Stop()
}

// TimeToRTP convert time in milliseconds to RTP time
func TimeToRTP(timeMS, clockRate uint64) uint32 {
	return uint32(timeMS * clockRate / 1000)
}
