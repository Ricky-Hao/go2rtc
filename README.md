# go2rtc patch set

This checkout maintains the patches for `Ricky-Hao/go2rtc`. Generate the Go application in a separate Git checkout using the exact upstream commit and ordered groups in [`manifest.json`](manifest.json). The original fork history and user guide are available on [`master` at `48ff711`](https://github.com/Ricky-Hao/go2rtc/tree/48ff711437d05852d9e1cce1aa788ed2330c2fc5).

## Generate and test the application

From the patch checkout root, with Git, Python 3.9+ and Go 1.25.14 on `PATH`, use a new application directory under `.work/`.

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

The root [devcontainer](.devcontainer/devcontainer.json) uses a Go 1.25 image, Git, Python and the tools used in the Coder workspace. Envbuilder builds it with the `vscode` remote user and `/workspaces` as the workspace parent. The Coder template provides the OpenCode/acpx module and any Git authentication. Generation and local tests need no application credentials, Compose or running cameras.

In an attached Coder VS Code session, run the generation commands from the patch checkout and use **File → Open Folder** to open the generated `.work/application` for Go development. Use a second window for the patch checkout when maintaining the payloads. With local VS Code Dev Containers, open the patch checkout first and run **Dev Containers: Reopen in Container**, then open the generated folder inside that container.

An existing fallback workspace can use a Go 1.25.14 archive for its OS and architecture. Verify its SHA-256 using the official download listing, unpack it under `.work/toolchain` and put its `go/bin` on `PATH` for that shell. The devcontainer is consumed when an authorized workspace build uses this checkout. [Official Go downloads](https://go.dev/dl/)

## Maintenance

See the [maintenance guide](docs/maintenance.md) for updating the base, editing groups and reviewing CI results. The [extraction record](docs/extraction.md) lists the migration's source commits and how to compare trees. [Status](docs/status.md) records current checks and limits. [Sources and licenses](SOURCES.md)
