package xiaomi

import (
	"errors"
	"net/http"
	"sync"
	"testing"

	pkgxiaomi "github.com/AlexxIT/go2rtc/pkg/xiaomi"
	"github.com/stretchr/testify/require"
)

type fakeCloudClient struct {
	mu        sync.Mutex
	responses []fakeCloudResponse
	requests  int
	onRequest func()
}

type fakeCloudResponse struct {
	body []byte
	err  error
}

func (c *fakeCloudClient) Request(string, string, string, map[string]string) ([]byte, error) {
	if c.onRequest != nil {
		c.onRequest()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.requests++
	if len(c.responses) == 0 {
		return nil, errors.New("unexpected cloud request")
	}
	res := c.responses[0]
	c.responses = c.responses[1:]
	return res.body, res.err
}

func (c *fakeCloudClient) requestCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requests
}

func resetCloudAuthTest(t *testing.T) {
	oldClouds := clouds
	oldTokens := tokens
	oldNewCloud := newCloud

	clouds = nil
	tokens = map[string]string{"user": "token"}

	t.Cleanup(func() {
		clouds = oldClouds
		tokens = oldTokens
		newCloud = oldNewCloud
	})
}

func TestCloudRequestRefreshesCloudOnUnauthorized(t *testing.T) {
	resetCloudAuthTest(t)

	stale := &fakeCloudClient{
		responses: []fakeCloudResponse{
			{err: &pkgxiaomi.StatusError{StatusCode: http.StatusUnauthorized, Status: "401 Unauthorized"}},
		},
	}
	refreshed := &fakeCloudClient{
		responses: []fakeCloudResponse{
			{body: []byte(`{"ok":true}`)},
		},
	}
	created := []cloudClient{stale, refreshed}
	newCloud = func(userID string) (cloudClient, error) {
		require.Equal(t, "user", userID)
		cloud := created[0]
		created = created[1:]
		return cloud, nil
	}

	body, err := cloudRequest("user", "cn", "/v2/device/miss_get_vendor", "{}")
	require.NoError(t, err)
	require.Equal(t, []byte(`{"ok":true}`), body)
	require.Equal(t, 1, stale.requestCount())
	require.Equal(t, 1, refreshed.requestCount())
	require.Empty(t, created)
	require.Same(t, refreshed, clouds["user"])
}

func TestCloudRequestUsesAlreadyRefreshedCloud(t *testing.T) {
	resetCloudAuthTest(t)

	refreshed := &fakeCloudClient{
		responses: []fakeCloudResponse{
			{body: []byte(`{"ok":true}`)},
		},
	}
	stale := &fakeCloudClient{
		responses: []fakeCloudResponse{
			{err: &pkgxiaomi.StatusError{StatusCode: http.StatusUnauthorized, Status: "401 Unauthorized"}},
		},
		onRequest: func() {
			cloudsMu.Lock()
			clouds["user"] = refreshed
			cloudsMu.Unlock()
		},
	}
	clouds = map[string]cloudClient{"user": stale}
	newCloud = func(string) (cloudClient, error) {
		return nil, errors.New("unexpected cloud login")
	}

	body, err := cloudRequest("user", "cn", "/v2/device/miss_get_vendor", "{}")
	require.NoError(t, err)
	require.Equal(t, []byte(`{"ok":true}`), body)
	require.Equal(t, 1, stale.requestCount())
	require.Equal(t, 1, refreshed.requestCount())
	require.Same(t, refreshed, clouds["user"])
}

func TestCloudRequestDoesNotRefreshOnNonUnauthorizedError(t *testing.T) {
	resetCloudAuthTest(t)

	staleErr := errors.New("cloud unavailable")
	stale := &fakeCloudClient{
		responses: []fakeCloudResponse{
			{err: staleErr},
		},
	}
	created := []cloudClient{stale}
	newCloud = func(string) (cloudClient, error) {
		cloud := created[0]
		created = created[1:]
		return cloud, nil
	}

	body, err := cloudRequest("user", "cn", "/v2/device/miss_get_vendor", "{}")
	require.ErrorIs(t, err, staleErr)
	require.Nil(t, body)
	require.Equal(t, 1, stale.requestCount())
	require.Empty(t, created)
	require.Same(t, stale, clouds["user"])
}

func TestCloudRequestRetriesOnlyOnceOnUnauthorized(t *testing.T) {
	resetCloudAuthTest(t)

	unauthorized := &pkgxiaomi.StatusError{StatusCode: http.StatusUnauthorized, Status: "401 Unauthorized"}
	stale := &fakeCloudClient{
		responses: []fakeCloudResponse{
			{err: unauthorized},
		},
	}
	refreshed := &fakeCloudClient{
		responses: []fakeCloudResponse{
			{err: unauthorized},
		},
	}
	created := []cloudClient{stale, refreshed}
	newCloud = func(string) (cloudClient, error) {
		cloud := created[0]
		created = created[1:]
		return cloud, nil
	}

	body, err := cloudRequest("user", "cn", "/v2/device/miss_get_vendor", "{}")
	require.ErrorIs(t, err, pkgxiaomi.ErrUnauthorized)
	require.Nil(t, body)
	require.Equal(t, 1, stale.requestCount())
	require.Equal(t, 1, refreshed.requestCount())
	require.Empty(t, created)
	require.Same(t, refreshed, clouds["user"])
}

func TestSetUserTokenClearsCachedCloud(t *testing.T) {
	resetCloudAuthTest(t)

	stale := &fakeCloudClient{}
	clouds = map[string]cloudClient{"user": stale}

	setUserToken("user", "new-token")

	require.Equal(t, "new-token", tokens["user"])
	require.NotContains(t, clouds, "user")
}
