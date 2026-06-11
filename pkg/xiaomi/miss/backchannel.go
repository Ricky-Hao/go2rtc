package miss

import (
	"fmt"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/opus"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
)

func (p *Producer) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	if err := p.stream.session.startSpeaker(); err != nil {
		return err
	}
	// TODO: check this!!!
	time.Sleep(time.Second)

	sourceCodec, err := backchannelCodec(codec, track.Codec)
	if err != nil {
		return err
	}

	sender := core.NewSender(media, sourceCodec)

	switch sourceCodec.Name {
	case core.CodecPCMA:
		var buf []byte

		if p.stream.session.speakerCodec() == codecPCM {
			dst := &core.Codec{Name: core.CodecPCML, ClockRate: 8000}
			transcode := pcm.Transcode(dst, sourceCodec)

			sender.Handler = func(pkt *rtp.Packet) {
				buf = append(buf, transcode(pkt.Payload)...)
				const size = 2 * 8000 * 0.040 // 16bit 40ms
				for len(buf) >= size {
					p.Send += size
					_ = p.stream.session.writeAudio(codecPCM, buf[:size])
					buf = buf[size:]
				}
			}
		} else {
			sender.Handler = func(pkt *rtp.Packet) {
				buf = append(buf, pkt.Payload...)
				const size = 8000 * 0.040 // 8bit 40 ms
				for len(buf) >= size {
					p.Send += size
					_ = p.stream.session.writeAudio(codecPCMA, buf[:size])
					buf = buf[size:]
				}
			}
		}
	case core.CodecOpus:
		if p.stream.session.speakerCodec() == codecOPUS {
			var buf []byte
			sender.Handler = func(pkt *rtp.Packet) {
				if buf == nil {
					buf = pkt.Payload
				} else {
					// convert two 20ms to one 40ms
					buf = opus.JoinFrames(buf, pkt.Payload)
					p.Send += len(buf)
					_ = p.stream.session.writeAudio(codecOPUS, buf)
					buf = nil
				}
			}
		} else {
			sender.Handler = func(pkt *rtp.Packet) {
				p.Send += len(pkt.Payload)
				_ = p.stream.session.writeAudio(codecOPUS, pkt.Payload)
			}
		}
	default:
		return fmt.Errorf("xiaomi: unsupported backchannel codec: %s", sourceCodec.Name)
	}

	sender.HandleRTP(track)
	p.Senders = append(p.Senders, sender)
	return nil
}

func backchannelCodec(negotiated, track *core.Codec) (*core.Codec, error) {
	if isSpecificCodec(negotiated) {
		return negotiated, nil
	}
	if isSpecificCodec(track) {
		return track, nil
	}
	return nil, fmt.Errorf("xiaomi: unsupported backchannel codec: %s", codecName(track))
}

func isSpecificCodec(codec *core.Codec) bool {
	if codec == nil {
		return false
	}
	switch codec.Name {
	case "", core.CodecAny, core.CodecAll:
		return false
	}
	return true
}

func codecName(codec *core.Codec) string {
	if codec == nil {
		return "<nil>"
	}
	return codec.Name
}
