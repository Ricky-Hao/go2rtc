# Extracting the original fork

The original migration used the common base `dc1685e9cf7a8c349181f20a1b4a44825ed394c5` from `AlexxIT/go2rtc` and fork master `48ff711437d05852d9e1cce1aa788ed2330c2fc5` from `Ricky-Hao/go2rtc`. Upstream master was checked separately at `ac84bb863b61ffe5a0195a8fc745f3219e4fefa9`. See the status record for the current upstream base and replay. [Original official base](https://github.com/AlexxIT/go2rtc/commit/dc1685e9cf7a8c349181f20a1b4a44825ed394c5) · [Original master](https://github.com/Ricky-Hao/go2rtc/commit/48ff711437d05852d9e1cce1aa788ed2330c2fc5) · [Status](status.md)

## Net deltas

`git log --reverse --topo-order <base>..<fork>` gives the original history. `git diff --name-status <base> <fork>` gives the original 18 net paths. The current manifest lists two active groups and one upstream replacement. [Manifest](../manifest.json)

| Group | Source delta |
| --- | --- |
| [CS2 media buffer](../patches/cs2-media-buffer/README.md) | `pkg/xiaomi/miss/cs2/conn.go` from the fork delta. The queue change landed in `b231f78`. |
| [Xiaomi MISS](../patches/xiaomi-miss/README.md) | The 14 Xiaomi paths from the shared-session rework through `44b79b4855a383f2ea78c2542c08dde1d10ffd55`. Adds dropped-channel restart in `session.go` and `session_manager_test.go` through PR #1's merge `5f2281d5bc86d292f1ae6a252823d52a41d410da`. Adds receive-only audio in `producer.go`, `producer_test.go` and the Xiaomi guide through PR #3's merge `48ff711` after `95a92fc`. |
| [Xiaomi cloud auth](../patches/xiaomi-cloud-auth/README.md) | The cloud request client, tests and status handling through PR #2's merge `95a92fc9a36c42799d1f305701d46b4429ec48ed`, after `5f2281d5`. The shared file uses context common to the official base and the Xiaomi MISS tree. |

The shared-session delta incorporates the demux work (`ada7280`), TTL and close hardening (`941ef98`, `df75537`), the cache and classifier extraction (`ae7c4c3`, `90ed39a`), lifecycle, channel and URL fixes (`147722f`, `b413212`, `89b3fc5`, `cef242e`) and the shared-audio changes (`70c79d3`, `c8bf4fb`, `e5a475d`, `0c977de`). The historical implementation plan is retained at `internal/xiaomi/MISS_REFACTOR_IMPLEMENTATION_PLAN.md`, blob `97807acb2267d28c7f53082ce93fc616b096ba49`.

The three-group replay combined the former `miss-shared-session`, `miss-channel-reconnect` and `miss-receive-only` deltas after the separate CS2 patch. The current replay uses the official CS2 fix. Both Xiaomi payloads are exported from independent checkouts at the pinned base. [CS2 replacement](../patches/cs2-media-buffer/README.md) · [Seven-group payloads](https://github.com/Ricky-Hao/go2rtc/tree/84c6b07d5ce3d84cdbf99efce1c5668c7451344a/patches)

## Workflow lineage

The three-group replay used the three official application workflows. The retired `legacy-workflow` group carried only the three `false` guards for `docker-master`, `docker-hardware` and `docker-rockchip` from the original fork. All application workflow files in the current generated tree match its pinned official base, including the newer `go-test.yml`. [Original fork workflow change](https://github.com/Ricky-Hao/go2rtc/commit/f7f939b25301b36495905539691c79da30143892)

| Application path | Official base and three-group blob | Original fork and seven-group blob | Role |
| --- | --- | --- | --- |
| `.github/workflows/build.yml` | `6015efa020d00215ee7adc0475e6dc6815475faf` | `4cdb92b746db1d5d773f29b3a389e070de899c75` | Historical `master` and `v*` tag build workflow. |
| `.github/workflows/test.yml` | `5d9e7e25658c68722925dd273cbcd11708cecc0c` | `5d9e7e25658c68722925dd273cbcd11708cecc0c` | Historical manual tests, including experimental jobs. |
| `.github/workflows/gh-pages.yml` | `65005306297573824bfa5841b4783c92dbd5c658` | `65005306297573824bfa5841b4783c92dbd5c658` | Historical manual Pages workflow. |

The patch checkout's `.github/workflows/patchset.yml` tests and builds a separate generated tree for `patches` and its PRs. It preserves the 13 binary targets, archive names and filenames. The generated checkout stays local or within the CI job. [Active patch workflow](../.github/workflows/patchset.yml)

## Compare the trees

The seven-group replay at patch revision `84c6b07d5ce3d84cdbf99efce1c5668c7451344a` produced `b52e05ebec5f675f575f68a1841f402904052927`. It matched all 550 paths in the original fork tree `b8224b35c418b3d2ee95948e16d7c99354a197cb` and added `AGENTS.md` with mode `100644` and blob `72cc4200c0a2a0e908381f9f7e0ea01fe35fb36e` through the former `contributor-guidance` group. [Seven-group acceptance](status.md#seven-group-acceptance)

The three-group replay at the original official base produced `dd187f00503eed953f3301374468493dfbd0fab1`. Compared with the seven-group replay, it removes the contributor guide and restores the official `build.yml` blob with mode `100644`. The other 549 paths match by path, mode, object type and object ID, including all 423 Go files, 42 of which are test files, and the user docs and licenses. Go development and patch export instructions now live in the patch checkout. [Application development](maintenance.md#application-development)

To reproduce that historical comparison, use the full three-group replay for `app` and the seven-group replay at the archived revision for `old_app`. The raw diff should show the two paths above. The two `ls-tree` lines should match exactly.

```bash
app=.work/application
old_app=.work/seven-group-application
base=$(git -C "$app" rev-parse HEAD)
tree=$(git -C "$app" write-tree)
old_tree=$(git -C "$old_app" write-tree)
test "$base" = dc1685e9cf7a8c349181f20a1b4a44825ed394c5
test "$tree" = dd187f00503eed953f3301374468493dfbd0fab1
test "$old_tree" = b52e05ebec5f675f575f68a1841f402904052927
git -C "$app" diff --exit-code
git -C "$app" diff --cached --check
old_objects=$(git -C "$old_app" rev-parse --path-format=absolute --git-path objects)
GIT_ALTERNATE_OBJECT_DIRECTORIES="$old_objects" \
  git -C "$app" diff --raw --no-renames --no-abbrev "$old_tree" "$tree" --
git -C "$app" ls-tree "$base" -- .github/workflows/build.yml
git -C "$app" ls-tree "$tree" -- .github/workflows/build.yml
```

To compare the three-group replay with original `master`, use a local checkout with `48ff711437d05852d9e1cce1aa788ed2330c2fc5` as the alternate object source and that commit as the old revision in the raw diff. Their only difference is the restored workflow.
