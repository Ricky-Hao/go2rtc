package miss

import (
	"encoding/hex"
	"io"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/stretchr/testify/require"
)

func TestProbeAddsDefaultSharedAudioMedia(t *testing.T) {
	s := newSession(newFakeClient(), "key", nil)
	st := newTestStream(s)
	st.ch <- testH264Packet(t)

	medias, err := probe(st, true)
	require.NoError(t, err)
	require.Len(t, medias, 3)
	require.Equal(t, core.KindVideo, medias[0].Kind)
	require.Equal(t, core.KindAudio, medias[1].Kind)
	require.Equal(t, core.DirectionRecvonly, medias[1].Direction)
	require.Equal(t, core.CodecPCMA, medias[1].Codecs[0].Name)
	require.Equal(t, core.KindAudio, medias[2].Kind)
	require.Equal(t, core.DirectionSendonly, medias[2].Direction)
	require.Equal(t, core.CodecPCMA, medias[2].Codecs[0].Name)
}

func TestProbeUsesPCMARecvDefaultForOpusTalk(t *testing.T) {
	client := newFakeClient()
	client.speakerCodec = codecOPUS
	s := newSession(client, "key", nil)
	st := newTestStream(s)
	st.ch <- testH264Packet(t)

	medias, err := probe(st, true)
	require.NoError(t, err)
	require.Len(t, medias, 3)
	require.Equal(t, core.CodecPCMA, medias[1].Codecs[0].Name)
	require.Equal(t, core.CodecOpus, medias[2].Codecs[0].Name)
}

func TestProbeDoesNotAddSharedAudioMediaWhenDisabled(t *testing.T) {
	s := newSession(newFakeClient(), "key", nil)
	st := newTestStream(s)
	st.ch <- testH264Packet(t)

	medias, err := probe(st, false)
	require.NoError(t, err)
	require.Len(t, medias, 1)
	require.Equal(t, core.KindVideo, medias[0].Kind)
}

func TestProbeReusesObservedSharedAudioCodec(t *testing.T) {
	s := newSession(newFakeClient(), "key", nil)
	st0 := newTestStream(s)
	st0.ch <- &Packet{
		CodecID: codecPCMU,
		Flags:   1 << 3,
		Payload: []byte{1, 2, 3, 4},
	}
	st0.ch <- testH264Packet(t)

	medias0, err := probe(st0, true)
	require.NoError(t, err)
	require.Len(t, medias0, 3)
	require.Equal(t, core.CodecPCMU, medias0[1].Codecs[0].Name)
	require.Equal(t, uint32(16000), medias0[1].Codecs[0].ClockRate)
	require.Equal(t, core.CodecPCMA, medias0[2].Codecs[0].Name)

	st1 := newTestStream(s)
	st1.ch <- testH264Packet(t)

	medias1, err := probe(st1, true)
	require.NoError(t, err)
	require.Len(t, medias1, 3)
	require.Equal(t, core.CodecPCMU, medias1[1].Codecs[0].Name)
	require.Equal(t, uint32(16000), medias1[1].Codecs[0].ClockRate)
	require.Equal(t, core.CodecPCMA, medias1[2].Codecs[0].Name)
}

func TestProducerStartHandlesSharedAudioCodecs(t *testing.T) {
	s := newSession(newFakeClient(), "key", nil)
	st := newTestStream(s)

	pcmuCodec := &core.Codec{Name: core.CodecPCMU, ClockRate: 8000}
	pcmlCodec := &core.Codec{Name: core.CodecPCML, ClockRate: 8000}
	pcmuReceiver := core.NewReceiver(&core.Media{Kind: core.KindAudio}, pcmuCodec)
	pcmlReceiver := core.NewReceiver(&core.Media{Kind: core.KindAudio}, pcmlCodec)

	p := &Producer{
		Connection: core.Connection{
			Receivers: []*core.Receiver{pcmuReceiver, pcmlReceiver},
		},
		stream: st,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- p.Start()
	}()

	st.ch <- &Packet{CodecID: codecPCMU, Sequence: 1, Payload: []byte{1, 2, 3, 4}}
	st.ch <- &Packet{CodecID: codecPCM, Sequence: 2, Payload: []byte{1, 0, 2, 0}}
	closeWhenDrained(st)

	require.ErrorIs(t, <-errCh, io.EOF)
	require.Equal(t, 1, pcmuReceiver.Packets)
	require.Equal(t, 1, pcmlReceiver.Packets)
}

func TestProducerStartTranscodesSharedPCMToDefaultAudioReceiver(t *testing.T) {
	s := newSession(newFakeClient(), "key", nil)
	st := newTestStream(s)

	pcmaCodec := &core.Codec{Name: core.CodecPCMA, ClockRate: 8000}
	pcmaReceiver := core.NewReceiver(&core.Media{Kind: core.KindAudio}, pcmaCodec)

	p := &Producer{
		Connection: core.Connection{
			Receivers: []*core.Receiver{pcmaReceiver},
		},
		stream: st,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- p.Start()
	}()

	st.ch <- &Packet{CodecID: codecPCMU, Sequence: 1, Payload: []byte{0x7F, 0x7E, 0x7D, 0x7C}}
	st.ch <- &Packet{CodecID: codecPCM, Sequence: 2, Payload: []byte{1, 0, 2, 0}}
	closeWhenDrained(st)

	require.ErrorIs(t, <-errCh, io.EOF)
	require.Equal(t, 2, pcmaReceiver.Packets)
}

func TestProducerStartResamplesSameSharedAudioCodec(t *testing.T) {
	s := newSession(newFakeClient(), "key", nil)
	st := newTestStream(s)

	pcmaCodec := &core.Codec{Name: core.CodecPCMA, ClockRate: 8000}
	pcmaReceiver := core.NewReceiver(&core.Media{Kind: core.KindAudio}, pcmaCodec)
	var packets []*core.Packet
	pcmaReceiver.AppendChild(&core.Node{
		Input: func(pkt *core.Packet) {
			clone := *pkt
			clone.Payload = append([]byte(nil), pkt.Payload...)
			packets = append(packets, &clone)
		},
	})

	p := &Producer{
		Connection: core.Connection{
			Receivers: []*core.Receiver{pcmaReceiver},
		},
		stream: st,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- p.Start()
	}()

	st.ch <- &Packet{CodecID: codecPCMA, Sequence: 1, Flags: 1 << 3, Payload: []byte{0xD5, 0xD6, 0xD7, 0xD8}}
	st.ch <- &Packet{CodecID: codecPCMA, Sequence: 2, Flags: 1 << 3, Payload: []byte{0xD9, 0xDA, 0xDB, 0xDC}}
	closeWhenDrained(st)

	require.ErrorIs(t, <-errCh, io.EOF)
	require.Len(t, packets, 2)
	require.Len(t, packets[0].Payload, 2)
	require.Equal(t, uint32(2), packets[1].Timestamp)
}

func newTestStream(s *session) *stream {
	st := &stream{
		session: s,
		ch:      make(chan *Packet, 10),
		done:    make(chan struct{}),
	}
	st.deadline.Store(time.Time{})
	return st
}

func closeWhenDrained(st *stream) {
	go func() {
		for len(st.ch) > 0 {
			time.Sleep(time.Millisecond)
		}
		st.close()
	}()
}

func testH264Packet(t *testing.T) *Packet {
	t.Helper()

	avcc, err := hex.DecodeString("000000196764001fac2484014016ec0440000003004000000c23c60c920000000568ee32c8b0000000d365")
	require.NoError(t, err)

	return &Packet{
		CodecID: codecH264,
		Payload: annexb.DecodeAVCC(avcc, true),
	}
}
