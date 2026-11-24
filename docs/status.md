# Patch set status

## Current upstream replay

The manifest pins official commit `85495f41d961ad166c5631de32df664b46bb8588` with tree `2318a2ec9ecb182ed39304acdd95c66771f0290b`. `cs2-media-buffer` records official ancestor `6fe6e24b92d5a2bdf7345891792471f20382e08e` and applies zero patches. `xiaomi-miss` and `xiaomi-cloud-auth` apply independently and in that order in the full replay. [Manifest](../manifest.json) · [Official base](https://github.com/AlexxIT/go2rtc/commit/85495f41d961ad166c5631de32df664b46bb8588) · [CS2 replacement](../patches/cs2-media-buffer/README.md)

The full application index tree is `bc4ec36ff100f983ace813ee5818ff4a1674d5d0`. It changes exactly 16 paths from the official base. All four upstream workflow files, the CS2 transport and tests, the new Xiaomi model guide and the dependency files match official objects and modes. Compared with the prior three-group application, 121 of the 125 changed paths match the new upstream. The other four carry the cache type move, the new `in` and `tw` regions in the shared Xiaomi entry point, and the shared session's close-before-reconnect ordering and regression test. The Xiaomi user guide, implementation plan and license match the prior replay.

### Local acceptance on 2026-11-24

The Go checks used Go 1.27.4 on Linux amd64, `CGO_ENABLED=0`, `GOTOOLCHAIN=local` and cached modules. The official Linux amd64 archive was checked against SHA-256 `7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840`. Upstream's `go.mod` declares Go 1.26.0 and its application build workflow selects Go 1.27. [Official downloads](https://go.dev/dl/)

| Check | Result |
| --- | --- |
| `check` and `apply` | Exit 0 from clean checkouts for all three selections and the full manifest. The CS2 selection reproduced the official tree. MISS alone produced `06aa4789c9c9d48130689c100265e7d5984773bf` and cloud auth alone produced `d29f4c7dee3e6bb5e21c5e68cd98451ca35fb7ee`. |
| `test --only <id>` | Exit 0. The CS2 package built and `TestPatchsetQueueOverflow` passed. All 40 named MISS tests and 5 cloud auth tests were discovered and passed in their own closures. |
| `test --tier all --plan` and `test --tier all` | Exit 0. Eight checks planned and executed in the full application, including CS2 overflow, the 45 named Xiaomi tests and nine related packages that run tests. |
| `go test -v -mod=readonly -count=1 -timeout=15s ./internal/xiaomi ./pkg/xiaomi/...` | Exit 0. The existing contracts' 45 tests and three upstream CS2 worker tests ran. |
| `go vet -mod=readonly -copylocks ./internal/xiaomi ./pkg/xiaomi/...` | Exit 0. The shared-secret call path and the Xiaomi session and cloud lock ownership were reviewed. |
| `python3 -m unittest discover -s tools` | Exit 0, 13 tests. |
| `go build -mod=readonly -ldflags "-s -w" -trimpath -o <binary> .` | Exit 0 for Linux amd64 after an independent CI build replay reproduced the validated application tree. `go version -m` confirmed Go 1.27.4, Linux amd64 and CGO disabled. |
| `go test -json -mod=readonly -count=1 -timeout=15s -skip '^TestDiscovery$' ./...` | Exit 0 for both the new official base and the full replay. Respectively 37 and 39 packages ran tests and passed. Two FFmpeg device cases skipped themselves in both. |

The quick suite excluded `pkg/mdns.TestDiscovery`. The runs used a private network namespace with multicast blocked and no external route. An isolated dummy interface supplied a non-loopback address for the in-process WebRTC peers. The first official-base run with only loopback exited 1 because `pkg/webrtc.TestKeyframeRequest` could not connect. That test passed with the dummy interface, as did both full runs under identical settings.

The patch workflow passed `actionlint` and shell syntax checks. It still builds for 13 targets with the same artifacts, triggers and permissions. The devcontainer JSON and Dockerfile passed static checks and the `golang:1.27.4-bookworm` tag was confirmed in the official image registry. This replay has local validation. Hosted CI, the other 12 target builds, Envbuilder, real camera picture and audio under dual-channel or high-bitrate load, and Xiaomi cloud interactions have no new run evidence. [Patch workflow](../.github/workflows/patchset.yml) · [Devcontainer](../.devcontainer/devcontainer.json)

## Three-group local acceptance

The previous manifest pinned `dc1685e9cf7a8c349181f20a1b4a44825ed394c5` and replayed `cs2-media-buffer`, `xiaomi-miss` and `xiaomi-cloud-auth` in that order. Each group applied independently. The full replay produced application tree `dd187f00503eed953f3301374468493dfbd0fab1`. Compared with the seven-group replay, it removed the application `AGENTS.md` and restored `build.yml` to the official base. The other 549 tracked paths matched, including the Go sources, tests, user docs and licenses. [Tree comparison](extraction.md#compare-the-trees)

On 2026-11-24, `check` and `apply` passed for each of the three groups and for the full manifest from clean checkouts at the pinned base. Each independent replay passed `test --only <id>`. CS2 compiled, and all 40 named Xiaomi MISS tests and 5 cloud auth tests were discovered and passed.

The full replay passed all seven checks in `test --tier all`, including the 45 named Go tests and tests in nine related media and streaming packages. The 13 Python maintenance tests, `go build -mod=readonly -ldflags "-s -w" -trimpath`, and Git whitespace checks for both checkouts passed. The Go checks used Go 1.25.14 on Linux amd64, with CGO disabled, cached modules and a loopback-only network namespace with multicast blocked.

The three-way full-suite comparison and 13-target build matrix below are from the seven-group migration. The three-group acceptance covers the local contracts and Linux amd64 build.

## Seven-group acceptance

This acceptance record covers patch revision `84c6b07d5ce3d84cdbf99efce1c5668c7451344a` from 2026-11-24 and its application tree `b52e05ebec5f675f575f68a1841f402904052927`. The generated application matched all tracked objects and modes in original fork `master` at `48ff711437d05852d9e1cce1aa788ed2330c2fc5` and added the Go contributor guide. [Archived patch revision](https://github.com/Ricky-Hao/go2rtc/tree/84c6b07d5ce3d84cdbf99efce1c5668c7451344a)

### Local migration evidence

Go checks used Go 1.25.14 on Linux amd64 and `CGO_ENABLED=0`. The public archive `go1.25.14.linux-amd64.tar.gz` was verified with SHA-256 `a21ae5633a269bcd7e90cf767e48225633795e99d831742cbf3397064fee7712`. [Official archive checksums](https://go.dev/dl/)

| Check | Result |
| --- | --- |
| `python3 -m unittest discover -s tools` | Exit 0, 13 tests. Covers order and transitive selection, base and checkout validation, atomic conflict handling, payload inventory, replacement ancestry and test discovery. |
| `check` and `apply` | Exit 0 from the official base for all seven groups and for every group's closure in a clean checkout. The full replay produced `b52e05ebec5f675f575f68a1841f402904052927`. |
| `test --tier all --plan` | Exit 0, nine check entries printed. |
| `test --tier all` | Exit 0, nine contracts executed. All 45 named Xiaomi/MISS tests were discovered and passed. Nine additional local packages ran media or streaming tests. The CS2 transport compiled. |
| `test --only <id>` | Exit 0 for the focused checks in each of the seven closure replays. |
| `go build -mod=readonly -ldflags "-s -w" -trimpath -o <binary> .` | Exit 0 for all 13 workflow targets in the generated application. `go version -m` confirmed the target settings in each binary. |

The broader `go test -mod=readonly -count=1 -skip '^TestDiscovery$' ./...` exited 1 in the generated tree, original fork and unpatched official base under the same Go settings. All three had the same 10 failing packages, 16 failing test cases and 7 compile or vet diagnostics. The command excluded `pkg/mdns.TestDiscovery`, which sends device-discovery multicast packets.

| Packages | Existing failures |
| --- | --- |
| `internal/ffmpeg`, `internal/streams`, `pkg/aac`, `pkg/creds`, `pkg/hap/tlv8` | Assertions in the existing parser, stream, AAC, credential and TLV8 tests. |
| `pkg/core`, `pkg/hap/camera`, `pkg/hap/hds` | Test compilation refers to undefined `StripUserinfo`, `SetupEndpoints`, and `Client`, respectively. |
| `pkg/ivideon`, `pkg/tuya` | Go vet reports non-constant `fmt.Errorf` format strings. |

The CI workflow passed `actionlint` and shell syntax checks. Its shell steps reproduced the same tree in two local checkouts and built Linux amd64 from `.work/application`. The static comparison matched all 13 original target settings, artifact names and filenames. Both jobs use the event SHA and the manifest base with read-only repository access and credential persistence disabled. Artifact uploads are limited to pushes and manual runs on the fork's `patches` branch. [CI workflow](../.github/workflows/patchset.yml)

The devcontainer passed local JSON, build context and Dockerfile shell checks. The official Go image tag was confirmed. The original local validation covered the devcontainer files and local CI reproduction. A fresh Coder Envbuilder build, real camera runs and Xiaomi cloud requests were outside that validation. [Devcontainer configuration](../.devcontainer/devcontainer.json)

### Hosted baseline

The maintainer reconfirmed on 2026-11-24 at 13:42 and 16:17 UTC that run `45377247241`, attempt 1 of `.github/workflows/patchset.yml` in `Ricky-Hao/go2rtc`, completed with `success` for the `push` to `patches` at `84c6b07d5ce3d84cdbf99efce1c5668c7451344a`. This run covers the seven-group baseline. [GitHub Actions run](https://github.com/Ricky-Hao/go2rtc/actions/runs/45377247241)
