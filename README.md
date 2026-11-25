# go2rtc patch set

This checkout maintains two independent patch groups and one upstream replacement record for `Ricky-Hao/go2rtc`. Generate the Go application in a separate Git checkout for application development, tests and builds. The manifest pins the official upstream commit and lists the groups in application order. [Manifest](manifest.json) · [Original fork history and user guide at `48ff711`](https://github.com/Ricky-Hao/go2rtc/tree/48ff711437d05852d9e1cce1aa788ed2330c2fc5)

- `xiaomi-miss` lets two channels of one camera stream over one physical connection, restarts dropped channels, and supports receive-only audio. [Xiaomi MISS](patches/xiaomi-miss/README.md)
- `xiaomi-cloud-auth` refreshes and retries Xiaomi cloud requests after HTTP 401. [Xiaomi cloud authentication](patches/xiaomi-cloud-auth/README.md)
- `cs2-media-buffer` uses the upstream overflow fix. The 10-packet control and 100-packet media queues drop incoming packets when full and the transport keeps running. [CS2 replacement and checks](patches/cs2-media-buffer/README.md)

## Generate and test the application

From the patch checkout root, with Git, Python 3.9+ and Go 1.27.4 on `PATH`, use a new application directory under `.work/`. Set `app` to an unused path if an earlier replay already exists there.

```sh
app=.work/application
base=$(python3 -c 'from tools.patchset import load_manifest; print(load_manifest()["base"])')
git init "$app"
git -C "$app" fetch --no-tags https://github.com/AlexxIT/go2rtc.git "$base"
git -C "$app" switch --detach "$base"

python3 tools/patchset.py list
python3 tools/patchset.py check --target "$app"
python3 tools/patchset.py apply --target "$app"
python3 tools/patchset.py test --target "$app" --tier all --plan
CGO_ENABLED=0 GOTOOLCHAIN=local python3 tools/patchset.py test --target "$app" --tier all
(cd "$app" && CGO_ENABLED=0 GOTOOLCHAIN=local go build -mod=readonly -ldflags "-s -w" -trimpath .)
```

`check` preflights the ordered patches against a clean checkout. `apply` leaves the result staged. Use `git -C "$app" diff --cached` to inspect it, or open its `README.md` and `internal/xiaomi/README.md` for application and camera setup. `test --plan` checks the entry paths and shows the commands' names. Omit `--plan` to execute the contracts. Use `--only <id>` to select a group and its dependencies and start each new replay in a fresh checkout. For patch tooling itself, run `python3 -m unittest discover -s tools` at the patch root.

## Coder and VS Code

The root devcontainer uses Go 1.27.4 on Debian Bookworm, Git, Python and the tools used in the Coder workspace. Envbuilder builds it with the `vscode` remote user and `/workspaces` as the workspace parent. The Coder template provides the OpenCode/acpx module and any Git authentication. Generation and local tests need no application credentials, Compose or running cameras. [Devcontainer](.devcontainer/devcontainer.json)

In an attached Coder VS Code session, run the generation commands from the patch checkout and use **File → Open Folder** to open the generated `.work/application` for Go development. Use a second window for the patch checkout when maintaining the payloads. With local VS Code Dev Containers, open the patch checkout first and run **Dev Containers: Reopen in Container**, then open the generated folder inside that container.

An existing fallback workspace can use a Go 1.27.4 archive for its OS and architecture. Verify its SHA-256 using the official download listing, unpack it under an unused `.work/` directory and put its `go/bin` on `PATH` for that shell. The devcontainer is consumed when an authorized workspace build uses this checkout. [Official Go downloads](https://go.dev/dl/)

## Maintenance

The patch workflow tests the generated application and builds `go2rtc_linux_arm64` for pushes to `patches` or `patches-*` tags, PRs targeting `patches`, and manual runs. In `Ricky-Hao/go2rtc`, `patches` branch pushes and manual runs on that branch provide the binary as an Actions artifact. Matching tag pushes also provide the artifact and publish it as the sole binary asset in a GitHub Release after validation. [Patch workflow](.github/workflows/patchset.yml)

See how to develop in the generated application, write changes back to a group, update the base and review CI results. [Maintenance guide](docs/maintenance.md)

The extraction record gives the source commits and tree comparison commands. Status records current checks and the three- and seven-group acceptance history. [Extraction record](docs/extraction.md) · [Status](docs/status.md) · [Sources and licenses](SOURCES.md)
