package miss

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPacketClassifierHeaderChannelNeedsBothValues(t *testing.T) {
	c := newPacketClassifier()

	require.Equal(t, uint8(0), c.Classify(&Packet{
		ChannelOK: true,
		Channel:   1,
		Timestamp: 1,
	}))
	require.Equal(t, uint8(0), c.Classify(&Packet{
		ChannelOK: true,
		Channel:   0,
		Timestamp: 2,
	}))
	require.Equal(t, uint8(1), c.Classify(&Packet{
		ChannelOK: true,
		Channel:   1,
		Timestamp: 3,
	}))
}

func TestPacketClassifierFlagsChannelNeedsBothValues(t *testing.T) {
	c := newPacketClassifier()

	require.Equal(t, uint8(0), c.Classify(&Packet{
		FlagsChannel: 1,
		Timestamp:    1,
	}))
	require.Equal(t, uint8(0), c.Classify(&Packet{
		FlagsChannel: 0,
		Timestamp:    2,
	}))
	require.Equal(t, uint8(1), c.Classify(&Packet{
		FlagsChannel: 1,
		Timestamp:    3,
	}))
}

func TestPacketClassifierAssignResolution(t *testing.T) {
	c := newPacketClassifier()

	c.assignResolution(640 * 360)
	require.Equal(t, uint8(0), c.resolutions[640*360])

	c.assignResolution(1920 * 1080)
	require.Equal(t, uint8(0), c.resolutions[1920*1080])
	require.Equal(t, uint8(1), c.resolutions[640*360])

	c.assignResolution(1280 * 720)
	require.Equal(t, uint8(0), c.resolutions[1920*1080])
	require.Equal(t, uint8(1), c.resolutions[1280*720])
	require.Equal(t, uint8(1), c.resolutions[640*360])
}

func TestPacketClassifierTimestampTieBreaksToChannelZero(t *testing.T) {
	c := newPacketClassifier()
	c.lastTS = [2]uint64{100, 100}
	c.tsInit = [2]bool{true, true}

	require.Equal(t, uint8(0), c.Classify(&Packet{Timestamp: 110}))
}

func TestPacketClassifierTimestampIgnoresBackwardsTimestamp(t *testing.T) {
	c := newPacketClassifier()
	c.lastTS = [2]uint64{200, 100}
	c.tsInit = [2]bool{true, true}

	require.Equal(t, uint8(1), c.Classify(&Packet{Timestamp: 150}))
}
