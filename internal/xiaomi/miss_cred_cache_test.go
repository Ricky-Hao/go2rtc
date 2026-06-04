package xiaomi

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMissCredCache(t *testing.T) {
	now := time.Unix(1000, 0)
	cache := newMissCredCache(time.Minute, func() time.Time { return now })
	key := missCredKey{userID: "user", region: "cn", did: "did1"}
	cred := missCred{clientPublic: "cpub", vendor: "cs2"}

	cache.Set(key, cred)

	got, ok := cache.Get(key)
	require.True(t, ok)
	require.Equal(t, cred, got)

	now = now.Add(time.Minute)
	got, ok = cache.Get(key)
	require.False(t, ok)
	require.Empty(t, got)
	require.Empty(t, cache.items)
}

func TestMissCredCacheKeySeparatesCredentials(t *testing.T) {
	cache := newMissCredCache(time.Minute, nil)
	key := missCredKey{userID: "user", region: "cn", did: "did1"}
	cred := missCred{clientPublic: "cpub"}

	cache.Set(key, cred)

	for _, other := range []missCredKey{
		{userID: "other", region: "cn", did: "did1"},
		{userID: "user", region: "de", did: "did1"},
		{userID: "user", region: "cn", did: "did2"},
	} {
		_, ok := cache.Get(other)
		require.False(t, ok)
	}
}

func TestGetMissURLUsesCachedCredentials(t *testing.T) {
	old := missCreds
	t.Cleanup(func() { missCreds = old })

	now := time.Unix(1000, 0)
	missCreds = newMissCredCache(time.Minute, func() time.Time { return now })
	missCreds.Set(missCredKey{userID: "user", region: "cn", did: "did1"}, missCred{
		clientPublic:  "cpub",
		clientPrivate: "cpriv",
		devicePublic:  "dpub",
		sign:          "sign",
		vendor:        "tutk",
		uid:           "uid",
	})

	u, err := url.Parse("xiaomi://user:cn@192.168.1.2?did=did1&model=test")
	require.NoError(t, err)

	rawURL, err := getMissURL(u)
	require.NoError(t, err)

	got, err := url.Parse(rawURL)
	require.NoError(t, err)
	query := got.Query()
	require.Equal(t, "cpub", query.Get("client_public"))
	require.Equal(t, "cpriv", query.Get("client_private"))
	require.Equal(t, "dpub", query.Get("device_public"))
	require.Equal(t, "sign", query.Get("sign"))
	require.Equal(t, "tutk", query.Get("vendor"))
	require.Equal(t, "uid", query.Get("uid"))
}

func TestGetMissURLDoesNotSetEmptyCachedUID(t *testing.T) {
	old := missCreds
	t.Cleanup(func() { missCreds = old })

	missCreds = newMissCredCache(time.Minute, nil)
	missCreds.Set(missCredKey{userID: "user", region: "cn", did: "did1"}, missCred{
		clientPublic:  "cpub",
		clientPrivate: "cpriv",
		devicePublic:  "dpub",
		sign:          "sign",
		vendor:        "cs2",
	})

	u, err := url.Parse("xiaomi://user:cn@192.168.1.2?did=did1&uid=old")
	require.NoError(t, err)

	rawURL, err := getMissURL(u)
	require.NoError(t, err)

	got, err := url.Parse(rawURL)
	require.NoError(t, err)
	require.Equal(t, "old", got.Query().Get("uid"))
}
