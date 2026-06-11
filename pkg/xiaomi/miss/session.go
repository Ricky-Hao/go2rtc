package miss

import (
	"errors"
	"io"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// session manages a single MISS client connection that can serve multiple
// streams (channels). For dual-channel cameras, both channel 0 and channel 1
// share the same underlying UDP/TCP session to avoid exhausting the camera's
// limited connection slots.
type session struct {
	client  sessionClient
	key     string // cache key: host|did
	manager *sessionManager

	mu      sync.Mutex
	streams map[*stream]struct{}
	state   sessionState
	reason  shutdownReason

	// startedMask tracks which channels have been started (bit 0 = ch0, bit 1 = ch1).
	startedMask  uint8
	restartMask  uint8
	quality      [2]string // remembered quality per channel
	mediaAudio   bool
	audioStarted bool

	workerOnce sync.Once
	workerDone chan struct{}

	speakerOnce sync.Once
	speakerErr  error

	audioCodec *core.Codec
	classifier *packetClassifier
}

// stream represents a single channel's view of a session. It receives
// packets dispatched by the session worker and provides a ReadPacket
// interface compatible with what the Producer expects.
type stream struct {
	session *session
	channel uint8
	audio   audioMode
	ch      chan *Packet

	closeOnce sync.Once
	deadline  atomic.Value // time.Time
	done      chan struct{}
}

type sessionClient interface {
	Protocol() string
	Version() string
	IsDafangLike() bool
	StartMedia(channel, quality, audio string) error
	StartMediaDual(quality0, quality1, audio string) error
	StartAudio() error
	StopMedia() error
	StartSpeaker() error
	SpeakerCodec() uint32
	WriteAudio(codecID uint32, payload []byte) error
	ReadPacket() (*Packet, error)
	RemoteAddr() net.Addr
	SetDeadline(t time.Time) error
	Close() error
}

type sessionState uint8

const (
	sessionActive sessionState = iota
	sessionClosing
	sessionClosed
)

type shutdownReason uint8

const (
	shutdownReadError shutdownReason = iota
	shutdownNoStreams
)

const (
	stopMediaTimeout  = time.Second
	workerStopTimeout = time.Second
)

var errSessionClosing = errors.New("miss: session is closing")

type audioMode uint8

const (
	audioDisabled audioMode = iota
	audioVideoStart
	audioCommandStart
)

func parseAudioMode(raw string) audioMode {
	switch raw {
	case "0":
		return audioDisabled
	case "2":
		return audioCommandStart
	default:
		return audioVideoStart
	}
}

func (m audioMode) enabled() bool {
	return m != audioDisabled
}

// sessionKey builds a cache key from the URL. Two URLs pointing to the same
// camera (same host and did) will share a session.
func sessionKey(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	key := u.Host
	if did := u.Query().Get("did"); did != "" {
		key += "|" + did
	}
	return key, nil
}

func newSession(client sessionClient, key string, manager *sessionManager) *session {
	return &session{
		client:     client,
		key:        key,
		manager:    manager,
		streams:    make(map[*stream]struct{}),
		state:      sessionActive,
		workerDone: make(chan struct{}),
		classifier: newPacketClassifier(),
	}
}

// openStream creates a new stream for the given channel on this session.
func (s *session) openStream(channel uint8, audio audioMode) (*stream, error) {
	s.mu.Lock()
	st, err := s.openStreamLocked(channel, audio)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	s.startWorker()
	return st, nil
}

// openStreamLocked creates a new stream while s.mu is held.
func (s *session) openStreamLocked(channel uint8, audio audioMode) (*stream, error) {
	if s.state != sessionActive {
		return nil, errSessionClosing
	}

	st := &stream{
		session: s,
		channel: channel,
		audio:   audio,
		ch:      make(chan *Packet, 100),
		done:    make(chan struct{}),
	}
	st.deadline.Store(time.Time{})

	s.streams[st] = struct{}{}
	return st, nil
}

func (s *session) startWorker() {
	s.workerOnce.Do(func() {
		go s.worker()
	})
}

// startMedia sends the appropriate VideoStart command. If only one channel is
// active, it sends a single-channel command. If both channels are active, it
// sends a dual-channel command.
func (s *session) startMedia(channel uint8, quality, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := int(channel)
	if ch > 1 {
		ch = 0
	}

	bit := channelBit(uint8(ch))

	// Already started this channel.
	if s.startedMask&bit != 0 {
		if s.restartMask&bit != 0 {
			s.quality[ch] = quality
			return s.restartMediaLocked(uint8(ch))
		}
		if !s.mediaAudio && s.audioEnabledLocked() {
			return s.enableAudioLocked()
		}
		if s.shouldStartAudioLocked() {
			s.startAudioLocked()
		}
		return nil
	}

	s.quality[ch] = quality
	other := ch ^ 1
	audio := s.audioParamLocked()

	// If the other channel is already started, upgrade to dual-channel.
	if s.startedMask&(1<<other) != 0 {
		err := s.client.StartMediaDual(
			s.quality[0], s.quality[1], audio,
		)
		if err != nil {
			return err
		}
		s.startedMask |= 1 << ch
		s.mediaAudio = audio != "0"
		if s.shouldStartAudioLocked() {
			s.startAudioLocked()
		}
		return nil
	}

	// Single channel start.
	chStr := "0"
	if channel == 1 {
		chStr = "1"
	}
	if err := s.client.StartMedia(chStr, quality, audio); err != nil {
		return err
	}
	s.startedMask |= 1 << ch
	s.mediaAudio = audio != "0"
	if s.shouldStartAudioLocked() {
		s.startAudioLocked()
	}
	return nil
}

func channelBit(channel uint8) uint8 {
	if channel == 1 {
		return 0b10
	}
	return 0b01
}

func (s *session) restartMediaLocked(channel uint8) error {
	bit := channelBit(channel)
	other := channel ^ 1
	audio := s.audioParamLocked()

	var err error
	if s.startedMask&channelBit(other) != 0 && s.hasChannelLocked(other) {
		err = s.client.StartMediaDual(s.quality[0], s.quality[1], audio)
	} else {
		err = s.client.StartMedia(channelString(channel), s.quality[channel], audio)
	}
	if err != nil {
		return err
	}

	s.restartMask &^= bit
	s.mediaAudio = audio != "0"
	s.audioStarted = false
	if s.shouldStartAudioLocked() {
		s.startAudioLocked()
	}
	return nil
}

func (s *session) audioEnabledLocked() bool {
	for st := range s.streams {
		if st.audio.enabled() {
			return true
		}
	}
	return false
}

func (s *session) audioCommandEnabledLocked() bool {
	for st := range s.streams {
		if st.audio == audioCommandStart {
			return true
		}
	}
	return false
}

func (s *session) hasChannelLocked(channel uint8) bool {
	for st := range s.streams {
		if st.channel == channel {
			return true
		}
	}
	return false
}

func (s *session) audioParamLocked() string {
	if s.audioEnabledLocked() {
		return "1"
	}
	return "0"
}

func (s *session) enableAudioLocked() error {
	if s.startedMask == 0 {
		return nil
	}

	var err error
	if s.startedMask == 0b11 {
		err = s.client.StartMediaDual(s.quality[0], s.quality[1], "1")
	} else {
		ch := uint8(0)
		if s.startedMask&0b10 != 0 {
			ch = 1
		}
		err = s.client.StartMedia(channelString(ch), s.quality[ch], "1")
	}
	if err != nil {
		return err
	}
	s.mediaAudio = true
	if s.shouldStartAudioLocked() {
		s.startAudioLocked()
	}
	return nil
}

func (s *session) shouldStartAudioLocked() bool {
	if s.audioStarted || s.client.IsDafangLike() || !s.audioEnabledLocked() {
		return false
	}
	return s.startedMask == 0b11 || s.audioCommandEnabledLocked()
}

func (s *session) startAudioLocked() {
	_ = s.client.StartAudio()
	s.audioStarted = true
}

func channelString(channel uint8) string {
	if channel == 1 {
		return "1"
	}
	return "0"
}

// worker is the read loop that dispatches packets to streams.
func (s *session) worker() {
	defer close(s.workerDone)

	for {
		_ = s.client.SetDeadline(time.Now().Add(10 * time.Second))

		pkt, err := s.client.ReadPacket()
		if err != nil {
			s.shutdown(shutdownReadError)
			return
		}

		s.dispatch(pkt)
	}
}

// dispatch routes a packet to the appropriate stream(s).
func (s *session) dispatch(pkt *Packet) {
	s.mu.Lock()
	if len(s.streams) == 0 {
		s.mu.Unlock()
		return
	}

	streams := make([]*stream, 0, len(s.streams))
	for st := range s.streams {
		streams = append(streams, st)
	}

	// Audio packets are broadcast to all streams.
	if isAudioCodec(pkt.CodecID) {
		s.mu.Unlock()
		for _, st := range streams {
			st.push(pkt)
		}
		return
	}

	// Single stream before dual mode: send all video to it, no classification needed.
	if len(streams) == 1 && s.startedMask != 0b11 {
		s.mu.Unlock()
		streams[0].push(pkt)
		return
	}

	if len(streams) == 1 {
		ch, ok := s.classifier.ClassifyKnown(pkt)
		s.mu.Unlock()
		if !ok || streams[0].channel == ch {
			streams[0].push(pkt)
		}
		return
	}

	// Multiple streams: classify and route. Already holding mu.
	ch := s.classifier.Classify(pkt)
	s.mu.Unlock()

	for _, st := range streams {
		if st.channel == ch {
			st.push(pkt)
		}
	}
}

// startSpeaker starts the speaker on the session (once).
func (s *session) startSpeaker() error {
	s.speakerOnce.Do(func() {
		s.speakerErr = s.client.StartSpeaker()
	})
	return s.speakerErr
}

// writeAudio sends audio data to the camera.
func (s *session) writeAudio(codecID uint32, payload []byte) error {
	return s.client.WriteAudio(codecID, payload)
}

// speakerCodec returns the speaker codec for the camera model.
func (s *session) speakerCodec() uint32 {
	return s.client.SpeakerCodec()
}

func (s *session) rememberAudioCodec(codec *core.Codec) *core.Codec {
	if codec == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.audioCodec == nil {
		s.audioCodec = codec.Clone()
	}
	return s.audioCodec.Clone()
}

func (s *session) cachedAudioCodec() *core.Codec {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.audioCodec == nil {
		return nil
	}
	return s.audioCodec.Clone()
}

func (s *session) defaultAudioCodec() *core.Codec {
	return &core.Codec{Name: core.CodecPCMA, ClockRate: 8000}
}

func (s *session) talkCodec() *core.Codec {
	switch s.speakerCodec() {
	case codecOPUS:
		return &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}
	default:
		return &core.Codec{Name: core.CodecPCMA, ClockRate: 8000}
	}
}

// removeStream removes a stream from the session. If no streams remain, the
// session is shut down.
func (s *session) removeStream(st *stream) {
	var shutdown bool

	s.mu.Lock()
	if _, ok := s.streams[st]; !ok {
		s.mu.Unlock()
		return
	}
	delete(s.streams, st)
	if len(s.streams) == 0 && s.state == sessionActive {
		s.state = sessionClosing
		s.reason = shutdownNoStreams
		shutdown = true
	} else if s.state == sessionActive && !s.hasChannelLocked(st.channel) {
		s.restartMask |= channelBit(st.channel)
	}
	s.mu.Unlock()

	st.close()

	if shutdown {
		s.completeShutdown(shutdownNoStreams, nil)
	}
}

// shutdown tears down the session, closing all streams and the client.
func (s *session) shutdown(reason shutdownReason) {
	streams, ok := s.beginShutdown(reason)
	if !ok {
		return
	}
	s.completeShutdown(reason, streams)
}

func (s *session) completeShutdown(reason shutdownReason, streams []*stream) {
	if s.manager != nil {
		s.manager.remove(s)
	}

	for _, st := range streams {
		st.close()
	}

	if reason == shutdownNoStreams {
		s.stopMedia()
	}
	_ = s.client.Close()

	if reason == shutdownNoStreams {
		s.waitWorkerDone()
	}

	s.finishShutdown()
}

func (s *session) beginShutdown(reason shutdownReason) ([]*stream, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != sessionActive {
		return nil, false
	}
	s.state = sessionClosing
	s.reason = reason

	streams := make([]*stream, 0, len(s.streams))
	for st := range s.streams {
		streams = append(streams, st)
	}
	s.streams = make(map[*stream]struct{})
	return streams, true
}

func (s *session) finishShutdown() {
	s.mu.Lock()
	s.state = sessionClosed
	s.mu.Unlock()
}

func (s *session) isActiveLocked() bool {
	return s.state == sessionActive
}

func (s *session) stopMedia() {
	_ = s.client.SetDeadline(time.Now().Add(stopMediaTimeout))

	done := make(chan struct{})
	go func() {
		_ = s.client.StopMedia()
		close(done)
	}()

	timer := time.NewTimer(stopMediaTimeout)
	defer timer.Stop()

	select {
	case <-done:
	case <-timer.C:
	}
}

func (s *session) waitWorkerDone() {
	timer := time.NewTimer(workerStopTimeout)
	defer timer.Stop()

	select {
	case <-s.workerDone:
	case <-timer.C:
	}
}

// --- stream methods ---

// ReadPacket reads the next packet for this stream, respecting deadlines.
func (st *stream) ReadPacket() (*Packet, error) {
	deadline, _ := st.deadline.Load().(time.Time)
	if deadline.IsZero() {
		select {
		case pkt := <-st.ch:
			return pkt, nil
		case <-st.done:
			return nil, io.EOF
		}
	}

	d := time.Until(deadline)
	if d <= 0 {
		// Check if there's already a packet available.
		select {
		case pkt := <-st.ch:
			return pkt, nil
		default:
			return nil, &net.OpError{Op: "read", Err: &timeoutError{}}
		}
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case pkt := <-st.ch:
		return pkt, nil
	case <-st.done:
		return nil, io.EOF
	case <-timer.C:
		return nil, &net.OpError{Op: "read", Err: &timeoutError{}}
	}
}

// SetDeadline sets a read deadline for this stream.
func (st *stream) SetDeadline(t time.Time) error {
	st.deadline.Store(t)
	return nil
}

// RemoteAddr returns the remote address of the underlying connection.
func (st *stream) RemoteAddr() net.Addr {
	return st.session.client.RemoteAddr()
}

// Close removes this stream from the session.
func (st *stream) Close() error {
	st.session.removeStream(st)
	return nil
}

func (st *stream) push(pkt *Packet) {
	select {
	case <-st.done:
		return
	default:
	}
	select {
	case st.ch <- pkt:
	default:
		// Drop packet if the buffer is full.
	}
}

func (st *stream) close() {
	st.closeOnce.Do(func() {
		close(st.done)
	})
}

// --- helpers ---

func isAudioCodec(codecID uint32) bool {
	switch codecID {
	case codecPCM, codecPCMU, codecPCMA, codecOPUS:
		return true
	}
	return false
}

// timeoutError implements the net.Error interface for deadline timeouts.
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }
