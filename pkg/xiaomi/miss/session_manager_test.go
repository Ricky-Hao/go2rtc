package miss

import (
	"errors"
	"io"
	"net"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeAddr string

func (a fakeAddr) Network() string { return "fake" }
func (a fakeAddr) String() string  { return string(a) }

type fakeClient struct {
	mu sync.Mutex

	dafang bool

	startMediaChannels  []string
	startMediaAudio     []string
	startMediaDual      int
	startMediaDualAudio []string
	startMediaDualErr   error
	stopMedia           int
	startSpeaker        int
	writeAudio          int
	closeCount          int
	speakerCodec        uint32

	closed    chan struct{}
	closeOnce sync.Once
	readErr   chan error
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		closed:  make(chan struct{}),
		readErr: make(chan error, 1),
	}
}

func (c *fakeClient) Protocol() string { return "fake" }
func (c *fakeClient) Version() string  { return "fake (model)" }
func (c *fakeClient) IsDafangLike() bool {
	return c.dafang
}

func (c *fakeClient) StartMedia(channel, _, audio string) error {
	c.mu.Lock()
	c.startMediaChannels = append(c.startMediaChannels, channel)
	c.startMediaAudio = append(c.startMediaAudio, audio)
	c.mu.Unlock()
	return nil
}

func (c *fakeClient) StartMediaDual(_, _, audio string) error {
	c.mu.Lock()
	c.startMediaDual++
	c.startMediaDualAudio = append(c.startMediaDualAudio, audio)
	err := c.startMediaDualErr
	c.mu.Unlock()
	return err
}

func (c *fakeClient) StopMedia() error {
	c.mu.Lock()
	c.stopMedia++
	c.mu.Unlock()
	return nil
}

func (c *fakeClient) StartSpeaker() error {
	c.mu.Lock()
	c.startSpeaker++
	c.mu.Unlock()
	return nil
}

func (c *fakeClient) SpeakerCodec() uint32 {
	if c.speakerCodec != 0 {
		return c.speakerCodec
	}
	return codecPCMA
}

func (c *fakeClient) WriteAudio(uint32, []byte) error {
	c.mu.Lock()
	c.writeAudio++
	c.mu.Unlock()
	return nil
}

func (c *fakeClient) ReadPacket() (*Packet, error) {
	select {
	case err := <-c.readErr:
		if err == nil {
			err = io.EOF
		}
		return nil, err
	case <-c.closed:
		return nil, io.EOF
	}
}

func (c *fakeClient) RemoteAddr() net.Addr { return fakeAddr("fake") }
func (c *fakeClient) SetDeadline(time.Time) error {
	return nil
}

func (c *fakeClient) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closeCount++
		c.mu.Unlock()
		close(c.closed)
	})
	return nil
}

func (c *fakeClient) counts() (stopMedia, closeCount, startMediaDual int, startMediaChannels []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopMedia, c.closeCount, c.startMediaDual, append([]string(nil), c.startMediaChannels...)
}

func (c *fakeClient) audioCommands() (startMediaAudio, startMediaDualAudio []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.startMediaAudio...), append([]string(nil), c.startMediaDualAudio...)
}

func TestSessionManagerAcquireReusesActiveSession(t *testing.T) {
	client := newFakeClient()
	manager := newSessionManager(func(string) (sessionClient, error) {
		return client, nil
	})

	s1, st1, err := manager.acquire("xiaomi://host?did=1", 0, true)
	require.NoError(t, err)
	s2, st2, err := manager.acquire("xiaomi://host?did=1", 1, true)
	require.NoError(t, err)
	require.Same(t, s1, s2)
	require.Equal(t, uint8(0), st1.channel)
	require.Equal(t, uint8(1), st2.channel)

	require.NoError(t, st1.Close())
	require.NoError(t, st2.Close())
	waitWorker(t, s1)
}

func TestSessionManagerDoesNotShareDafangLikeSession(t *testing.T) {
	var clients []*fakeClient
	manager := newSessionManager(func(string) (sessionClient, error) {
		client := newFakeClient()
		client.dafang = true
		clients = append(clients, client)
		return client, nil
	})

	s1, st1, err := manager.acquire("xiaomi://host?did=1", 0, true)
	require.NoError(t, err)
	s2, st2, err := manager.acquire("xiaomi://host?did=1", 1, true)
	require.NoError(t, err)
	require.NotSame(t, s1, s2)
	require.Len(t, clients, 2)

	require.NoError(t, st1.Close())
	require.NoError(t, st2.Close())
	waitWorker(t, s1)
	waitWorker(t, s2)
}

func TestSessionReadErrorShutdownSkipsStopMedia(t *testing.T) {
	client := newFakeClient()
	manager := newSessionManager(func(string) (sessionClient, error) {
		return client, nil
	})

	s, _, err := manager.acquire("xiaomi://host?did=1", 0, true)
	require.NoError(t, err)

	client.readErr <- errors.New("read failed")
	waitWorker(t, s)

	stopMedia, closeCount, _, _ := client.counts()
	require.Equal(t, 0, stopMedia)
	require.Equal(t, 1, closeCount)
}

func TestSessionNoStreamsShutdownStopsMedia(t *testing.T) {
	client := newFakeClient()
	manager := newSessionManager(func(string) (sessionClient, error) {
		return client, nil
	})

	s, st, err := manager.acquire("xiaomi://host?did=1", 0, true)
	require.NoError(t, err)
	require.NoError(t, st.Close())
	waitWorker(t, s)

	stopMedia, closeCount, _, _ := client.counts()
	require.Equal(t, 1, stopMedia)
	require.Equal(t, 1, closeCount)
}

func TestSessionStartMediaDualUpgrade(t *testing.T) {
	client := newFakeClient()
	manager := newSessionManager(func(string) (sessionClient, error) {
		return client, nil
	})

	s, st0, err := manager.acquire("xiaomi://host?did=1", 0, true)
	require.NoError(t, err)
	require.NoError(t, s.startMedia(0, "hd", "1"))
	_, st1, err := manager.acquire("xiaomi://host?did=1", 1, true)
	require.NoError(t, err)
	require.NoError(t, s.startMedia(1, "sd", "1"))

	_, _, startMediaDual, startMediaChannels := client.counts()
	require.Equal(t, []string{"0"}, startMediaChannels)
	require.Equal(t, 1, startMediaDual)
	require.Equal(t, uint8(0b11), s.startedMask)

	require.NoError(t, st1.Close())
	require.NoError(t, st0.Close())
	waitWorker(t, s)
}

func TestSessionStartMediaDualKeepsSharedAudioEnabled(t *testing.T) {
	client := newFakeClient()
	manager := newSessionManager(func(string) (sessionClient, error) {
		return client, nil
	})

	s, st0, err := manager.acquire("xiaomi://host?did=1", 0, true)
	require.NoError(t, err)
	require.NoError(t, s.startMedia(0, "hd", "1"))
	_, st1, err := manager.acquire("xiaomi://host?did=1", 1, false)
	require.NoError(t, err)
	require.NoError(t, s.startMedia(1, "sd", "0"))

	startMediaAudio, startMediaDualAudio := client.audioCommands()
	require.Equal(t, []string{"1"}, startMediaAudio)
	require.Equal(t, []string{"1"}, startMediaDualAudio)

	require.NoError(t, st1.Close())
	require.NoError(t, st0.Close())
	waitWorker(t, s)
}

func TestSessionStartMediaEnablesAudioForAlreadyStartedChannel(t *testing.T) {
	client := newFakeClient()
	manager := newSessionManager(func(string) (sessionClient, error) {
		return client, nil
	})

	s, st0, err := manager.acquire("xiaomi://host?did=1", 0, false)
	require.NoError(t, err)
	require.NoError(t, s.startMedia(0, "hd", "0"))
	_, st1, err := manager.acquire("xiaomi://host?did=1", 0, true)
	require.NoError(t, err)
	require.NoError(t, s.startMedia(0, "hd", "1"))

	startMediaAudio, startMediaDualAudio := client.audioCommands()
	require.Equal(t, []string{"0", "1"}, startMediaAudio)
	require.Empty(t, startMediaDualAudio)

	require.NoError(t, st1.Close())
	require.NoError(t, st0.Close())
	waitWorker(t, s)
}

func TestSessionDispatchClassifiesAfterDualStreamCloses(t *testing.T) {
	s := newSession(newFakeClient(), "key", nil)

	s.mu.Lock()
	st0, err := s.openStreamLocked(0, true)
	require.NoError(t, err)
	st1, err := s.openStreamLocked(1, true)
	require.NoError(t, err)
	s.startedMask = 0b11
	s.mu.Unlock()

	prime0 := &Packet{CodecID: 0xFFFF, FlagsChannel: 0}
	prime1 := &Packet{CodecID: 0xFFFF, FlagsChannel: 1}
	s.dispatch(prime0)
	s.dispatch(prime1)
	require.Same(t, prime0, <-st0.ch)
	require.Same(t, prime1, <-st1.ch)

	require.NoError(t, st0.Close())
	pkt0 := &Packet{CodecID: 0xFFFF, FlagsChannel: 0}
	pkt1 := &Packet{CodecID: 0xFFFF, FlagsChannel: 1}
	s.dispatch(pkt0)
	s.dispatch(pkt1)

	select {
	case got := <-st1.ch:
		require.Same(t, pkt1, got)
	default:
		t.Fatal("expected remaining channel stream to receive its own packet")
	}

	select {
	case got := <-st1.ch:
		t.Fatalf("unexpected extra packet: %#v", got)
	default:
	}

	st1.close()
}

func TestSessionDispatchKeepsSingleStreamBeforeClassifierIsReady(t *testing.T) {
	s := newSession(newFakeClient(), "key", nil)

	s.mu.Lock()
	st0, err := s.openStreamLocked(0, true)
	require.NoError(t, err)
	st1, err := s.openStreamLocked(1, true)
	require.NoError(t, err)
	s.startedMask = 0b11
	s.mu.Unlock()

	require.NoError(t, st0.Close())
	pkt1 := &Packet{CodecID: 0xFFFF, FlagsChannel: 1}
	s.dispatch(pkt1)

	select {
	case got := <-st1.ch:
		require.Same(t, pkt1, got)
	default:
		t.Fatal("expected remaining channel stream to receive packet before classifier is ready")
	}

	st1.close()
}

func TestSessionStartMediaDualFailureDoesNotMarkSecondChannelStarted(t *testing.T) {
	client := newFakeClient()
	client.startMediaDualErr = errors.New("dual failed")
	manager := newSessionManager(func(string) (sessionClient, error) {
		return client, nil
	})

	s, st0, err := manager.acquire("xiaomi://host?did=1", 0, true)
	require.NoError(t, err)
	require.NoError(t, s.startMedia(0, "hd", "1"))
	_, st1, err := manager.acquire("xiaomi://host?did=1", 1, true)
	require.NoError(t, err)
	require.Error(t, s.startMedia(1, "sd", "1"))
	require.Equal(t, uint8(0b01), s.startedMask)

	require.NoError(t, st1.Close())
	require.NoError(t, st0.Close())
	waitWorker(t, s)
}

func TestSessionManagerRedialsAfterReadError(t *testing.T) {
	var clients []*fakeClient
	manager := newSessionManager(func(string) (sessionClient, error) {
		client := newFakeClient()
		clients = append(clients, client)
		return client, nil
	})

	s1, _, err := manager.acquire("xiaomi://host?did=1", 0, true)
	require.NoError(t, err)
	clients[0].readErr <- errors.New("read failed")
	waitWorker(t, s1)

	s2, st2, err := manager.acquire("xiaomi://host?did=1", 0, true)
	require.NoError(t, err)
	require.NotSame(t, s1, s2)
	require.Len(t, clients, 2)

	require.NoError(t, st2.Close())
	waitWorker(t, s2)
}

func TestSessionManagerRedialsAfterLastStreamClose(t *testing.T) {
	var clients []*fakeClient
	manager := newSessionManager(func(string) (sessionClient, error) {
		client := newFakeClient()
		clients = append(clients, client)
		return client, nil
	})

	s1, st1, err := manager.acquire("xiaomi://host?did=1", 0, true)
	require.NoError(t, err)
	require.NoError(t, st1.Close())
	waitWorker(t, s1)

	s2, st2, err := manager.acquire("xiaomi://host?did=1", 0, true)
	require.NoError(t, err)
	require.NotSame(t, s1, s2)
	require.Len(t, clients, 2)

	require.NoError(t, st2.Close())
	waitWorker(t, s2)
}

func TestSessionManagerConcurrentAcquireClosesDuplicateClient(t *testing.T) {
	created := make(chan *fakeClient, 2)
	release := make(chan struct{})
	manager := newSessionManager(func(string) (sessionClient, error) {
		client := newFakeClient()
		created <- client
		<-release
		return client, nil
	})

	type result struct {
		session *session
		stream  *stream
		err     error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			s, st, err := manager.acquire("xiaomi://host?did=1", 0, true)
			results <- result{session: s, stream: st, err: err}
		}()
	}

	var clients []*fakeClient
	for range 2 {
		select {
		case client := <-created:
			clients = append(clients, client)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for duplicate dials")
		}
	}
	close(release)

	var got []result
	for range 2 {
		select {
		case res := <-results:
			require.NoError(t, res.err)
			got = append(got, res)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for acquire results")
		}
	}

	require.Same(t, got[0].session, got[1].session)
	closeCounts := 0
	for _, client := range clients {
		_, closeCount, _, _ := client.counts()
		closeCounts += closeCount
	}
	require.Equal(t, 1, closeCounts)

	require.NoError(t, got[0].stream.Close())
	require.NoError(t, got[1].stream.Close())
	waitWorker(t, got[0].session)
}

func TestParseChannel(t *testing.T) {
	query := url.Values{}
	channel, err := parseChannel(query)
	require.NoError(t, err)
	require.Equal(t, uint8(0), channel)

	query.Set("channel", "0")
	channel, err = parseChannel(query)
	require.NoError(t, err)
	require.Equal(t, uint8(0), channel)

	query.Set("channel", "1")
	channel, err = parseChannel(query)
	require.NoError(t, err)
	require.Equal(t, uint8(1), channel)

	query.Set("channel", "2")
	_, err = parseChannel(query)
	require.Error(t, err)
}

func TestDialRejectsMalformedURL(t *testing.T) {
	_, err := Dial("xiaomi://%zz")
	require.Error(t, err)
}

func waitWorker(t *testing.T, s *session) {
	t.Helper()

	select {
	case <-s.workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for worker")
	}
}
