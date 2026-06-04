# Xiaomi MISS refactor implementation plan

This branch is for implementing a behavior-preserving refactor of the Xiaomi
MISS path. The goal is to make session reuse, credential caching, packet
classification, and shutdown policy explicit and testable before making any
larger behavior changes.

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
- Do not make TTL configurable in the first pass.

## Key risks to address

### Reuse-after-shutdown

`getOrCreateSession` currently returns any cached session that is not
dafang-like. A session being closed must never be handed out again. The
session manager must check and update session state under one synchronization
boundary.

### Test seams

Session lifecycle cannot be tested cleanly while `session.client` is a concrete
`*Client` that performs real network I/O. The first code change should add a
minimal package-private client interface and fake client test seam.

### Classifier state split

Timestamp fallback currently depends on `lastTS` and `tsInit`, which are updated
after classification in `dispatch`. When extracting the classifier, all
classifier state and post-classify state updates must move together.

### Shutdown reason race

`shutdown` is protected by `closeOnce`, so the first caller wins. The refactor
must make that explicit:

- read error: skip `StopMedia`
- no streams left: best-effort `StopMedia`

The first shutdown reason should be captured once and used for the whole close.

## Proposed package structure

```text
internal/xiaomi/
  xiaomi.go
  miss_cred_cache.go
  miss_cred_cache_test.go

pkg/xiaomi/miss/
  client.go
  producer.go
  session.go
  session_manager.go
  session_manager_test.go
  classifier.go
  classifier_test.go
```

This keeps all new types package-private.

## Phase 1: Add test seam and lifecycle characterization tests

### Code changes

Introduce a small interface for the methods `session` actually uses:

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

`*Client` should satisfy this interface without behavior changes.

### Tests to add

Use a fake client in `pkg/xiaomi/miss` tests.

Pin the current intended lifecycle behavior:

1. active shared session is reused for a second stream
2. dafang-like session is not stored/reused
3. closing or closed session is not reused
4. read-error shutdown skips `StopMedia`
5. last-stream shutdown attempts bounded `StopMedia`

### Acceptance criteria

```bash
go test ./pkg/xiaomi/miss
```

passes with tests proving current lifecycle expectations.

## Phase 2: Extract session manager and explicit lifecycle state

### Code changes

Add a package-private manager:

```go
type sessionManager struct {
    mu       sync.Mutex
    sessions map[string]*session
}

func (m *sessionManager) getOrCreate(rawURL string) (*session, error)
func (m *sessionManager) remove(s *session)
```

Add explicit session state:

```go
type sessionState uint8

const (
    sessionActive sessionState = iota
    sessionClosing
    sessionClosed
)
```

Add explicit shutdown reason:

```go
type shutdownReason uint8

const (
    shutdownReadError shutdownReason = iota
    shutdownNoStreams
)
```

Rules:

- manager returns only `sessionActive` sessions
- shutdown marks the session closing before cleanup
- manager removes a session before slow cleanup
- `shutdownReadError` skips `StopMedia`
- `shutdownNoStreams` performs best-effort bounded `StopMedia`

### Important ordering

`sessionManager.getOrCreate` and `session.shutdown` must coordinate state and
map removal under the same manager mutex. Do not rely on a separate session
mutex for the reuse decision.

### Acceptance criteria

```bash
go test ./pkg/xiaomi/miss
go test -race ./pkg/xiaomi/miss
```

If `-race` reports a concurrent `StopMedia`/`Close` issue, adjust shutdown so
the connection is not closed while a command goroutine is still using it.

## Phase 3: Extract MISS credential cache

### Code changes

Move cache logic out of `xiaomi.go`:

```go
type missCredKey struct {
    UserID string
    Region string
    DID    string
}

type missCredCache struct {
    mu    sync.Mutex
    ttl   time.Duration
    now   func() time.Time
    items map[missCredKey]missCred
}

func (c *missCredCache) Get(key missCredKey) (missCred, bool)
func (c *missCredCache) Set(key missCredKey, cred missCred)
```

Keep the current 30-second TTL. Delete expired entries on access. Do not cache
legacy fallback results.

### Tests to add

1. fresh credentials are returned
2. expired credentials miss and are deleted
3. keys differ by user ID, region, and DID
4. cache `Set` computes expiration from the injectable clock

### Acceptance criteria

```bash
go test ./internal/xiaomi
```

passes without cloud/network calls.

## Phase 4: Extract packet classifier

### Code changes

Move classifier state and helper methods from `session` into `classifier.go`:

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

Preserve the current strategy order:

1. `hdr[28]` channel field after both channels have been observed
2. `(flags >> 24) & 0x01` after both channels have been observed
3. SPS resolution mapping
4. timestamp continuity
5. default channel `0`

The classifier should not own stream routing. `session.dispatch` should collect
streams, call the classifier only when multiple video streams are active, and
route the packet.

### Tests to add

Pin current quirks:

1. header channel is not trusted until both 0 and 1 are observed
2. flags channel is not trusted until both 0 and 1 are observed
3. resolution mapping assigns highest resolution to channel 0
4. with 3+ resolutions, max maps to 0 and all others map to 1
5. timestamp tie-break uses channel 0
6. timestamp fallback ignores backwards timestamps by treating them as far away

### Acceptance criteria

```bash
go test ./pkg/xiaomi/miss
```

passes after moving classifier logic.

## Phase 5: Cleanup and validation

### Cleanup

- Keep public behavior unchanged.
- Keep new types package-private.
- Remove obsolete comments tied to old structure.
- Avoid introducing broad logging or silent fallbacks.
- Keep `StopMedia` best-effort and bounded.

### Validation commands

Targeted validation:

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

The full suite currently has unrelated baseline failures outside Xiaomi, so the
targeted Xiaomi/MISS checks are the required gate for this branch.

## Implementation order

1. Add `sessionClient` seam and fake client tests.
2. Add `sessionManager`, explicit state, and `shutdownReason`.
3. Move global session map into the manager.
4. Extract credential cache and tests.
5. Extract packet classifier and tests.
6. Run targeted validation and inspect diff.

## Review checkpoints

Open review after each checkpoint if needed:

1. lifecycle seam + tests
2. session manager extraction
3. credential cache extraction
4. classifier extraction

The riskiest checkpoint is session manager extraction because it owns reconnect
correctness. That checkpoint should be reviewed before changing classifier
behavior or transport boundaries.
