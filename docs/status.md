# Patch set status

The manifest pins `dc1685e9cf7a8c349181f20a1b4a44825ed394c5`. All seven groups are active. The generated application matches the original fork's tracked modes and objects at `48ff711437d05852d9e1cce1aa788ed2330c2fc5`, with one added Go contributor guide. The trees, sources and workflow mapping are in the [extraction record](extraction.md).

## Local migration evidence

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

The devcontainer passed local JSON, build context and Dockerfile shell checks. The official Go image tag was confirmed. GitHub Actions, a fresh Coder Envbuilder build, and real camera or Xiaomi cloud requests were outside the local validation. [Devcontainer configuration](../.devcontainer/devcontainer.json)
