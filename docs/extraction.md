# Extracting the original fork

The verified merge base is `dc1685e9cf7a8c349181f20a1b4a44825ed394c5` from `AlexxIT/go2rtc`. The source is `48ff711437d05852d9e1cce1aa788ed2330c2fc5` from `Ricky-Hao/go2rtc` master. Upstream master was checked separately at `ac84bb863b61ffe5a0195a8fc745f3219e4fefa9`. The migration uses the common base. [Official base](https://github.com/AlexxIT/go2rtc/commit/dc1685e9cf7a8c349181f20a1b4a44825ed394c5) · [Original master](https://github.com/Ricky-Hao/go2rtc/commit/48ff711437d05852d9e1cce1aa788ed2330c2fc5)

## Net deltas

`git log --reverse --topo-order <base>..<fork>` gives the original history. `git diff --name-status <base> <fork>` gives the 18 net paths. The group inventory in the [manifest](../manifest.json) records exactly which paths each payload changes. The table maps source snapshots to the current groups.

| Group | Extraction |
| --- | --- |
| [CS2 media buffer](../patches/cs2-media-buffer/README.md) | `pkg/xiaomi/miss/cs2/conn.go` from the fork delta. The queue change landed in `b231f78`. |
| [Shared session](../patches/miss-shared-session/README.md) | The 14 remaining Xiaomi paths from the base through `44b79b4855a383f2ea78c2542c08dde1d10ffd55`. This contains the final form of dual-stream demux, shared lifecycle, the MISS credential cache, audio receive/talkback, their tests and docs. |
| [Channel reconnect](../patches/miss-channel-reconnect/README.md) | `session.go` and `session_manager_test.go` from the `44b79b4` tree to PR #1's merge `5f2281d5bc86d292f1ae6a252823d52a41d410da`. Requires shared session. |
| [Xiaomi cloud auth](../patches/xiaomi-cloud-auth/README.md) | The cloud request client, tests and status handling through PR #2's merge `95a92fc9a36c42799d1f305701d46b4429ec48ed`, after `5f2281d5`. The shared file uses context common to both the official and shared-session trees. |
| [Receive-only audio](../patches/miss-receive-only/README.md) | The `producer.go`, `producer_test.go` and Xiaomi guide changes through PR #3's merge `48ff711`, after `95a92fc`. Requires shared session. |
| [Legacy workflow](../patches/legacy-workflow/README.md) | The three disabled Docker jobs from `f7f939b25301b36495905539691c79da30143892`. |
| [Contributor guidance](../patches/contributor-guidance/README.md) | Adds the application Go `AGENTS.md` for this migration. |

The shared-session delta incorporates the demux work (`ada7280`), TTL and close hardening (`941ef98`, `df75537`), the cache and classifier extraction (`ae7c4c3`, `90ed39a`), lifecycle, channel and URL fixes (`147722f`, `b413212`, `89b3fc5`, `cef242e`) and the shared-audio changes (`70c79d3`, `c8bf4fb`, `e5a475d`, `0c977de`). The historical implementation plan is retained at `internal/xiaomi/MISS_REFACTOR_IMPLEMENTATION_PLAN.md`, blob `97807acb2267d28c7f53082ce93fc616b096ba49`.

## Workflow lineage

The three historical workflows are present in the generated tree, with the exact blobs below. The active patch checkout contains `.github/workflows/patchset.yml`.

| Original path | Blob in original and generated trees | Role |
| --- | --- | --- |
| `.github/workflows/build.yml` | `4cdb92b746db1d5d773f29b3a389e070de899c75` | The master and `v*` tag workflow built 13 binary artifacts. The new workflow preserves all 13 targets, archive names and binary filenames and builds the replayed tree for `patches` and its PRs. The original `docker-master`, `docker-hardware` and `docker-rockchip` guards remain `false` in the generated tree. |
| `.github/workflows/test.yml` | `5d9e7e25658c68722925dd273cbcd11708cecc0c` | Historical manual test workflow from the official base, including experimental jobs. Patch CI uses the local replay and Go test contracts. |
| `.github/workflows/gh-pages.yml` | `65005306297573824bfa5841b4783c92dbd5c658` | Historical manual Pages workflow from the official base. Deployment is outside patch CI. |

## Compare the trees

All 550 paths in the source tree `b8224b35c418b3d2ee95948e16d7c99354a197cb` match the generated tree by path, mode, object type and object ID, including all 42 existing Go test files, user documentation and licenses. The one additional path is `AGENTS.md` with mode `100644` and blob `72cc4200c0a2a0e908381f9f7e0ea01fe35fb36e`. The resulting tree is `b52e05ebec5f675f575f68a1841f402904052927`.

From the patch root after a full replay, set `fork_repo` to a local Git checkout that has the original fork commit. Git reads the fork objects through an alternate object directory for this command. The expected output is one addition for `AGENTS.md` with the mode and hash above.

```bash
fork=48ff711437d05852d9e1cce1aa788ed2330c2fc5
app=.work/application
tree=$(git -C "$app" write-tree)
git -C "$app" diff --exit-code
git -C "$app" diff --cached --check
fork_objects=$(git -C "$fork_repo" rev-parse --path-format=absolute --git-path objects)
GIT_ALTERNATE_OBJECT_DIRECTORIES="$fork_objects" \
  git -C "$app" diff --raw --no-renames --no-abbrev "$fork" "$tree" --
```
