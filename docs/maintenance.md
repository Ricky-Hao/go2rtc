# Maintaining the patch set

## Patch and application checkouts

Use the patch checkout for group diffs, test contracts, documentation, tooling and CI. Use the generated application checkout for development, tests and builds. It has its own Git index and starts at the manifest's full official commit. Reproduce a result by recording the patch checkout commit, manifest base and generated `git write-tree` ID. For a local review before committing, record the staged patch checkout tree as well. [Manifest](../manifest.json) · [Original fork snapshot and history](https://github.com/Ricky-Hao/go2rtc/commit/48ff711437d05852d9e1cce1aa788ed2330c2fc5)

The manifest is the only application order. `--only` and `--status` select entries and include their prerequisites in that order. Each active group carries a `changes.patch` and lists its expected changed paths in the manifest. Every entry has a `README.md` and a nonempty `tests.json`, including an upstream replacement. The tool checks the complete selection sequentially in a private Git index. After it succeeds, `check` checks the combined diff against the target, and `apply` stages that combined diff once. A conflict during preflight leaves the target working files and index untouched. Both operations require the base at `HEAD` and a clean target with no staged, unstaged or untracked changes.

Run the [README commands](../README.md#generate-and-test-the-application) to replay. `test` defaults to `focused`. `--tier all` also runs the related project checks. Named Go tests must be discovered by `go test -list` and then pass in an uncached `go test` run. `--plan` verifies the contract's file paths and lists the checks without running commands. Keep routine contract tests on fixtures, mocks or loopback and document separately authorized device coverage in the group guide.

## Application development

- Read the generated Go application's `README.md` and the relevant package docs before changing a module. The Xiaomi source options are in `internal/xiaomi/README.md`. Follow upstream code conventions and use the dependencies recorded in `go.mod` and `go.sum`.
- Use Go 1.27.4 and format changed Go files with `gofmt`. Run the owning package tests with `go test -mod=readonly -count=1`. For Xiaomi/MISS, test `./internal/xiaomi ./pkg/xiaomi/...`. Verify that targeted tests are discovered and run.
- Build from the application checkout with `CGO_ENABLED=0 GOTOOLCHAIN=local go build -mod=readonly -ldflags "-s -w" -trimpath .`. Keep runtime configuration, tokens and captures local. Camera, cloud and network discovery tests require authorization for the actual target.
- Record the generated index tree before editing. Stage the intended application changes and export the owning group from its correct predecessor. Verify the export in a fresh checkout at the pinned base using the steps below.

## Edit and export a group

1. Start a fresh application checkout at the manifest base. Apply the group's prerequisites with `--only` using the dependency IDs. With no prerequisites, start with the base itself. Save the starting tree with `before=$(git write-tree)` in the application checkout.
2. For an existing group, apply its current `changes.patch` with `git apply --index /path/to/patch-checkout/patches/<id>/changes.patch`. Edit the application, stage the intended code, tests and user docs, and save `after=$(git write-tree)`.
3. From that application checkout, export `git diff --binary --full-index --unified=3 --no-renames --no-ext-diff --no-textconv --no-color --src-prefix=a/ --dst-prefix=b/ "$before" "$after" --` into the group's `changes.patch`. When independent groups edit the same file, choose context that works in both the group's closure and full order. Xiaomi MISS and cloud auth share `internal/xiaomi/xiaomi.go`. [Cloud group](../patches/xiaomi-cloud-auth/README.md)
4. Update the group's path inventory, guide and test contract for the change. In fresh checkouts, check and apply the closure and the full manifest. Run the selected tests and check whitespace in both checkouts with `git diff --check` and `git diff --cached --check`. Include the relevant binary build in the review.

## Update the upstream base

1. Choose and fetch an exact commit from `AlexxIT/go2rtc` in a separate checkout. Use `git log` and `git diff` between the old and candidate base to review changes to patched paths and adjacent contracts. Record both SHAs and check each group against its replacement criteria.
2. Replay groups on the candidate in manifest order. Resolve conflicts in application source and export the remaining deltas with their prerequisite changes already applied. Recheck dependencies and touched paths. Keep unproven groups active and retain focused coverage of partially replaced behavior.
3. When upstream meets a group's replacement criteria, record the commit, behavioral evidence and any differences from the fork's policy. The `upstream` and `replaced` entries have `patch: null` and no payload paths. The replacement commit must be an ancestor of the selected base, and the contract still needs to validate the accepted behavior. Keep upstream history available so Git can verify ancestry.
4. Change the manifest base and payloads together. Replay the individual closures and the full manifest in clean checkouts. Run the real tests and build, compare the generated tree against the expected result, and update [status](status.md).

## Review, CI and release boundary

The active patch workflow validates pushes to `patches` and PRs targeting it at the event's exact `github.sha`. It replays the manifest on the official base in a separate `.work/application` checkout. The 13-target binary matrix independently reproduces the validated tree and builds with Go 1.27.4 and CGO disabled. On a `patches` push or manual run in `Ricky-Hao/go2rtc`, the binaries are available as Actions run artifacts. PR runs validate and build with read-only repository access. [Patch workflow](../.github/workflows/patchset.yml)

The generated checkout stays local or inside the CI job. Its application workflows come from the official base and are kept as source files. Carry changes back through the patch checkout for review. [Workflow lineage](extraction.md#workflow-lineage)

Review the staged migration and local evidence before landing the patch branch. Landing the patch branch, changing the default branch, triggering Actions manually, publishing a release, deploying or rebuilding a Coder workspace are separate actions. The patch workflow has no image build, Release or deploy job.
