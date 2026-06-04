# Xiaomi MISS refactor implementation plan

This branch is for implementing a behavior-preserving refactor of the Xiaomi
MISS path. The plan below is the merged result after independent reviews from
GPT-5.5, Opus 4.6, and Opus 4.7, followed by cross-review between the three
approaches.

## Current baseline

Branch: `refactor/xiaomi-miss-lifecycle`

Baseline commit:

```text
df75537 xiaomi: harden MISS session cleanup
```

Relevant files:

- `internal/xiaomi/xiaomi.go`
  - Xiaomi cloud URL construction
  - MISS credential cache and TTL
- `pkg/xiaomi/miss/producer.go`
  - Producer `Dial`
  - session open/start/probe flow
- `pkg/xiaomi/miss/session.go`
  - shared session map
  - session lifecycle
  - stream registration/removal
  - worker read loop
  - packet dispatch
  - dual-channel classifier state
  - shutdown and best-effort `StopMedia`

## Refactor goals

1. Preserve current user-facing behavior and URL format.
2. Make shared MISS session reuse safe by construction.
3. Make reconnect behavior auditable and testable.
4. Move dual-channel packet classification into an isolated component.
5. Add focused tests around the logic that caused recent regressions.
6. Avoid broad CS2/TUTK transport rewrites.

## Non-goals

- Do not change Xiaomi cloud API behavior.
- Do not change stream configuration syntax.
- Do not rewrite CS2/TUTK transport implementations.
- Do not change classifier strategy order unless new device traces justify it.
- Do not make the MISS credential TTL configurable in the first pass.
- Do not fix `startMedia` holding `session.mu` across network I/O in this
  refactor; document the hazard and keep the manager lock away from this path.

## Cross-review decisions

### Use a small `sessionClient` interface plus a client factory

The existing `Conn` interface is transport-level and does not expose the
higher-level methods used by sessions, such as `StartMedia`, `StartMediaDual`,
`StopMedia`, `ReadPacket() (*Packet, error)`, and `IsDafangLike`.

Use a package-private `sessionClient` interface and inject client construction
through the session manager:

```go
type clientFactory func(rawURL string) (sessionClient, error)
```

`*Client` must satisfy `sessionClient` without changing production behavior.

### Use lifecycle-safe acquire and stream registration

A plain `getOrCreate(rawURL) (*session, error)` is too weak because a returned
session can transition to closing before `Dial` calls `openStream`.

The manager should own a lifecycle-safe acquire operation:

```go
func (m *sessionManager) acquire(rawURL string, channel uint8) (*session, *stream, error)
```

This operation checks session state and registers the stream under the same
lifecycle boundary. Callers should not receive a session that cannot accept the
stream.

### Use one explicit session state machine

Do not keep `closeOnce` as a second source of truth beside a state field.

Use a package-private state machine guarded by `session.mu`:

```go
type sessionState uint8

const (
    sessionActive sessionState = iota
    sessionClosing
    sessionClosed
)
```

The first shutdown caller transitions `active -> closing`, captures the
shutdown reason, and owns cleanup. Later shutdown callers return without doing
cleanup.

### Keep classifier synchronization owned by `session`

`packetClassifier` should be a plain package-private struct with no internal
mutex. It is called only under `session.mu`. This preserves the current
synchronization model and avoids a second lock.

### Extract low-risk pure components before lifecycle work

The credential cache and classifier extractions are independent of session
lifecycle. Do them first to reduce the size and risk of the later manager
change.

Final phase order:

1. capture baseline
2. extract credential cache
3. extract packet classifier
4. add lifecycle test seam
5. introduce session manager and explicit lifecycle
6. cleanup and validation

## Key invariants

### Lock ordering

- Never dial or perform network I/O while holding `sessionManager.mu`.
- If both locks are needed, acquire `sessionManager.mu` before `session.mu`.
- Do not call `StartMedia`, `StartMediaDual`, `StopMedia`, `Close`, or
  `ReadPacket` while holding `sessionManager.mu`.
- `startMedia` may continue to hold `session.mu` across network I/O for now,
  but this must not be combined with manager locking.

### Shutdown policy

Use an explicit shutdown reason:

```go
type shutdownReason uint8

const (
    shutdownReadError shutdownReason = iota
    shutdownNoStreams
)
```

Rules:

- `shutdownReadError`: remove from manager, close streams, close client; skip
  `StopMedia`.
- `shutdownNoStreams`: remove from manager, close streams, attempt bounded
  best-effort `StopMedia`, then close client.
- The first shutdown reason wins.
- `StopMedia` must not silently race an immediate `Close`; tests must verify
  ordering in the normal no-stream shutdown path.

### Dialing and duplicate creation

The manager must not hold its mutex while dialing. Use double-check behavior:

1. lock manager
2. find an active reusable session if present
3. unlock
4. dial new client outside locks if needed
5. lock manager again
6. if another active session appeared, close the newly dialed client and use
   the existing session
7. otherwise install the new session

The lifecycle-safe acquire step must then register the stream only if the
session is still active.

## Phase 0: Capture baseline

Run targeted tests before changing code:

```bash
go test ./internal/xiaomi ./pkg/xiaomi/miss ./pkg/xiaomi/miss/cs2 ./pkg/tutk
```

Also run full suite once and record unrelated baseline failures:

```bash
go test ./...
```

The full suite currently has unrelated failures outside Xiaomi; capture the
list before this refactor so later results can distinguish pre-existing issues
from regressions.

## Phase 1: Extract MISS credential cache

### Code changes

Move cache logic out of `xiaomi.go` into `internal/xiaomi/miss_cred_cache.go`.

Use a typed key:

```go
type missCredKey struct {
    UserID string
    Region string
    DID    string
}
```

Use a small cache with injectable clock:

```go
type missCredCache struct {
    mu    sync.Mutex
    ttl   time.Duration
    now   func() time.Time
    items map[missCredKey]missCredEntry
}

type missCredEntry struct {
    cred      missCred
    expiresAt time.Time
}
```

Keep the current 30-second TTL. Store `expiresAt` in the cache entry, not in
`missCred`, so credential payloads cannot be accidentally reused with stale
expiration metadata.

Expected production global:

```go
var missCreds = newMissCredCache(missCredTTL, time.Now)
```

### Tests

Add `internal/xiaomi/miss_cred_cache_test.go`.

Required cases:

1. fresh credentials are returned
2. expired credentials miss and are deleted
3. keys differ by user ID, region, and DID
4. injectable clock controls expiration
5. legacy fallback results are not cached
6. cache hit restores all MISS URL query params:
   - `client_public`
   - `client_private`
   - `device_public`
   - `sign`
   - `vendor`
   - optional `uid`

### Acceptance

```bash
go test ./internal/xiaomi
```

## Phase 2: Extract packet classifier

### Code changes

Move classifier state and helper methods from `session` into
`pkg/xiaomi/miss/classifier.go`.

```go
type packetClassifier struct {
    hdrChanSeen   [2]bool
    flagsChanSeen [2]bool
    resolutions   map[uint32]uint8
    lastTS        [2]uint64
    tsInit        [2]bool
}

func newPacketClassifier() *packetClassifier
func (c *packetClassifier) Classify(pkt *Packet) uint8
```

`Classify` must include the current post-classify timestamp update:

```go
ch := c.classify(pkt)
c.lastTS[ch] = pkt.Timestamp
c.tsInit[ch] = true
return ch
```

Preserve the current strategy order exactly:

1. `hdr[28]` channel field after both channels have been observed
2. `(flags >> 24) & 0x01` after both channels have been observed
3. SPS resolution mapping
4. timestamp continuity
5. default channel `0`

`session.dispatch` remains responsible for:

- broadcasting audio
- sending all video to a single active stream
- calling the classifier only when multiple video streams are active
- routing the classified packet

### Tests

Add `pkg/xiaomi/miss/classifier_test.go`.

Required cases:

1. header channel is not trusted until both channels are observed
2. flags channel is not trusted until both channels are observed
3. highest resolution maps to channel 0
4. with 3+ resolutions, max maps to 0 and all others map to 1
5. timestamp tie-break uses channel 0
6. backwards timestamp is treated as far away
7. single-stream dispatch path does not call classifier

### Acceptance

```bash
go test ./pkg/xiaomi/miss
```

## Phase 3: Add lifecycle test seam

### Code changes

Introduce a package-private interface containing the methods used by `session`:

```go
type sessionClient interface {
    Protocol() string
    Version() string
    IsDafangLike() bool
    StartMedia(channel, quality, audio string) error
    StartMediaDual(quality0, quality1, audio string) error
    StopMedia() error
    StartSpeaker() error
    SpeakerCodec() uint32
    WriteAudio(codecID uint32, payload []byte) error
    ReadPacket() (*Packet, error)
    RemoteAddr() net.Addr
    SetDeadline(time.Time) error
    Close() error
}
```

Change session to hold:

```go
client sessionClient
```

Add a client factory to the future manager shape:

```go
type clientFactory func(rawURL string) (sessionClient, error)
```

Production factory wraps `NewClient`.

### Tests

Add a fake client in `pkg/xiaomi/miss` tests. It must support:

- controllable `IsDafangLike`
- programmable `ReadPacket` error for worker shutdown
- counters for `StartMedia`, `StartMediaDual`, `StopMedia`, and `Close`
- blocking `StopMedia` to test close ordering
- fake remote address

Required characterization tests:

1. active shared session is reused for second stream
2. dafang-like session is not stored/reused
3. read-error shutdown skips `StopMedia`
4. no-stream shutdown attempts bounded `StopMedia`
5. single-channel to dual-channel upgrade calls `StartMediaDual`
6. `StartMediaDual` failure does not mark the second channel started

### Acceptance

```bash
go test ./pkg/xiaomi/miss
```

## Phase 4: Introduce session manager and explicit lifecycle

### Code changes

Add `pkg/xiaomi/miss/session_manager.go`:

```go
type sessionManager struct {
    mu        sync.Mutex
    sessions map[string]*session
    newClient clientFactory
}
```

Use a package-level default:

```go
var defaultSessionManager = newSessionManager(defaultClientFactory)
```

Replace global `sessionMu` / `sessions` with manager-owned state.

The manager API should be lifecycle-safe:

```go
func (m *sessionManager) acquire(rawURL string, channel uint8) (*session, *stream, error)
func (m *sessionManager) remove(s *session)
```

Acquire rules:

1. parse session key
2. check for active reusable session under manager lock
3. do not dial under manager lock
4. do not store dafang-like sessions
5. if duplicate dialing races, close the losing client
6. register the stream only if the session is still active
7. do not return closing or closed sessions

Session state:

```go
type sessionState uint8

const (
    sessionActive sessionState = iota
    sessionClosing
    sessionClosed
)
```

Shutdown rules:

1. lock `session.mu`
2. if state is not active, return
3. set state to closing and capture shutdown reason
4. drain streams
5. unlock `session.mu`
6. remove from manager map
7. close drained streams
8. if reason is `shutdownNoStreams`, run bounded best-effort `StopMedia`
9. close client
10. mark state closed

Add `workerDone chan struct{}` so tests can deterministically assert worker
exit. Worker should close it before returning.

### StopMedia ordering

The normal no-stream path should not call `Close` before `StopMedia` returns or
the bounded timeout path intentionally proceeds. Tests should cover both fast
and blocked `StopMedia`.

If `StopMedia` times out, closing the client to unblock transport I/O is
acceptable, but the behavior must be explicit and tested with fake clients.

### Tests

Required lifecycle tests:

1. active shared session is reused
2. dafang-like session is not shared
3. closing/closed session is not reused
4. concurrent acquire during shutdown gets a new session or a clear retryable
   error handled inside `Dial`
5. concurrent acquire for the same key closes the losing client
6. read-error shutdown skips `StopMedia`
7. last-stream shutdown attempts bounded `StopMedia`
8. `Close` does not happen before `StopMedia` returns on the fast path
9. racing shutdown callers run cleanup once
10. first shutdown reason wins
11. redial after read-error creates a fresh session
12. worker exits before shutdown returns

### Acceptance

```bash
go test ./pkg/xiaomi/miss
go test -race ./pkg/xiaomi/miss
```

## Phase 5: Cleanup and validation

### Cleanup

- Remove obsolete global session map helpers.
- Remove classifier fields from `session`.
- Remove credential cache map/mutex free functions from `xiaomi.go`.
- Keep new types package-private.
- Add a short comment/TODO noting that `startMedia` still holds `session.mu`
  during network I/O and should be addressed separately if it becomes a real
  bottleneck.

### Validation

Targeted gate:

```bash
gofmt -w internal/xiaomi/*.go pkg/xiaomi/miss/*.go
go test ./internal/xiaomi ./pkg/xiaomi/miss ./pkg/xiaomi/miss/cs2 ./pkg/tutk
go test -race ./pkg/xiaomi/miss
git diff --check
```

Full-suite signal:

```bash
go test ./...
```

The targeted Xiaomi/MISS checks are the required gate for this branch because
the full suite currently has unrelated baseline failures outside Xiaomi.

## Commit plan

Use one reviewable commit per phase:

1. `xiaomi: extract MISS credential cache`
2. `xiaomi: extract MISS packet classifier`
3. `xiaomi: add MISS session test seam`
4. `xiaomi: refactor MISS session lifecycle`
5. `xiaomi: clean up MISS refactor`

The riskiest checkpoint is the session manager/lifecycle commit. It should be
reviewed before follow-up behavior changes such as moving `startMedia` network
I/O out of `session.mu`.
