# Contributing

The primary application lives in `sandbox-launcher/` and includes Python, R, and
general bioinformatics in one startup UI. Per-language source directories remain
as worker and compatibility-build inputs. Shared setup payloads live under `internal/`.

## Repository convention

Each sandbox should contain everything needed to reproduce its user-facing
download:

```text
<sandbox>/
├── assets/     # Application bundles, icons, and release artifacts
├── windows/    # Native Windows entry point
└── scripts/    # Reproducible build and packaging scripts
```

Shared launcher code and the vendored setup payloads live under `internal/`.
The payload for each sandbox is in
`internal/launcherbundle/bundles/<sandbox>/`. Keep it self-contained: it may
clone the skills repository into a generated project, but it must not read,
source, execute, or copy setup/runtime files from that clone.
When project defaults or runtime behavior change, update the vendored payload
here explicitly and test it as release source; do not add a runtime sync or
fallback to files in the skills clone.

Keep generated downloads in the sandbox's `assets/` directory and give them a
stable, descriptive name. A release must not depend on uncommitted files from a
developer's computer.

The macOS packaging scripts copy the corresponding vendored setup payload into
the application bundle. Windows executables embed the same payload at compile
time. A release must therefore remain usable when the skills clone contains no
launcher implementation files.

## Develop through GitHub

1. Fork or clone the GitHub repository.
2. Create a branch named for the sandbox and change, such as
   `python-sandbox/update-launcher`.
3. Modify the application and its packaging inputs inside that sandbox's
   directory only.
4. Build the downloadable artifact on the operating system targeted by the
   package. macOS `.app` and `.dmg` packages must be built on macOS.
5. Test both a clean installation and the complete first-run setup.
6. Commit the source changes and the finished release artifact together.
7. Open a pull request. Include the tested operating-system versions, artifact
   filename, and results of the checks below.
8. Merge the pull request before creating a GitHub Release, so every published
   download can be traced to a repository commit.

Do not commit credentials, signing certificates, notarization credentials,
developer-specific paths, or temporary build directories.

## Build and verify a sandbox deliverable

Run packaging commands from the repository root. Write test builds outside the
repository so an existing release artifact is not overwritten accidentally.

To build, verify, and update one stable artifact in `assets/`, use:

```bash
./scripts/create-release.sh <sandbox> <platform>
```

The sandbox may be `python`, `r`, or `bioinformatics`; the platform may be
`macos` or `windows`. For example:

```bash
./scripts/create-release.sh python macos
./scripts/create-release.sh bioinformatics windows
```

The helper builds in a temporary directory and replaces the selected stable
artifact only after verification succeeds. It does not create a Git tag or
publish a GitHub Release; follow the publishing section below after testing and
merging the artifact.

For the Python sandbox:

```bash
./python-sandbox/scripts/build-macos-dmg.sh /tmp/Python-Sandbox.dmg
hdiutil verify /tmp/Python-Sandbox.dmg
plutil -lint \
  "python-sandbox/assets/Python Sandbox.app/Contents/Info.plist"
bash -n \
  "python-sandbox/assets/Python Sandbox.app/Contents/MacOS/python-sandbox" \
  "python-sandbox/assets/Python Sandbox.app/Contents/Resources/Python Sandbox.command" \
  python-sandbox/scripts/build-macos-dmg.sh
```

For the R sandbox:

```bash
./r-sandbox/scripts/build-macos-dmg.sh /tmp/R-Sandbox.dmg
hdiutil verify /tmp/R-Sandbox.dmg
plutil -lint \
  "r-sandbox/assets/R Sandbox.app/Contents/Info.plist"
bash -n \
  "r-sandbox/assets/R Sandbox.app/Contents/MacOS/r-sandbox" \
  "r-sandbox/assets/R Sandbox.app/Contents/Resources/R Sandbox.command" \
  r-sandbox/scripts/build-macos-dmg.sh
```

For the bioinformatics sandbox:

```bash
./bioinformatics-sandbox/scripts/build-macos-dmg.sh \
  /tmp/Bioinformatics-Sandbox.dmg
hdiutil verify /tmp/Bioinformatics-Sandbox.dmg
plutil -lint \
  "bioinformatics-sandbox/assets/Bioinformatics Sandbox.app/Contents/Info.plist"
bash -n \
  "bioinformatics-sandbox/assets/Bioinformatics Sandbox.app/Contents/MacOS/bioinformatics-sandbox" \
  "bioinformatics-sandbox/assets/Bioinformatics Sandbox.app/Contents/Resources/Bioinformatics Sandbox.command" \
  bioinformatics-sandbox/scripts/build-macos-dmg.sh
```

### Windows executables

Windows launchers are native x86-64 GUI applications built with Go. Install
the pinned resource tool once:

```bash
go install github.com/tc-hib/go-winres@v0.3.3
```

Build test artifacts from the repository root:

```bash
./python-sandbox/scripts/build-windows-exe.sh /tmp/Python-Sandbox.exe
./r-sandbox/scripts/build-windows-exe.sh /tmp/R-Sandbox.exe
./bioinformatics-sandbox/scripts/build-windows-exe.sh \
  /tmp/Bioinformatics-Sandbox.exe
file /tmp/Python-Sandbox.exe /tmp/R-Sandbox.exe \
  /tmp/Bioinformatics-Sandbox.exe
```

The builds have no dependency on a skills repository revision. They embed the
tracked setup payload from `internal/launcherbundle/bundles/`.

The committed `windows/rsrc_windows_amd64.syso` files contain each executable's
icon, manifest, and version information. Regenerate them with the build scripts
whenever an icon or Windows metadata changes.

The initial Windows launcher copies itself to the matching
`code/Run <Sandbox>.exe` name and writes the selected agent to a companion file.
The copied executable recognizes the project-runner filename and directly starts
the bundled runtime launcher. Verify this behavior with:

```bash
go test ./...
bash scripts/test-self-contained.sh
bash scripts/test-daemon-autostart.sh
bash scripts/test-podman-workflow.sh
python3 scripts/test-apt-lock-retry.py
```

The self-contained test provides an empty mock skills clone and requires all
three setup flows to succeed. A clean generated project must contain the stable
user-facing runner filename, track it in Git, and reopen the existing sandbox
without rerunning setup.

Test every executable on a clean, supported 64-bit Windows 11 system. Verify the
unsupported-Windows and disabled-hypervisor failures, prerequisite diagnostics,
interactive prompts, Git clone, project setup, sandbox connection, extension
installation, and VS Code opening. Python and R tests require a working host Podman
machine; Bioinformatics Sandbox must work without host Podman. Verify Podman
build/run/exec inside each sandbox; no launcher requires Docker Desktop.

Then mount the DMG and verify that:

- the Finder window contains the application and an `Applications` shortcut;
- dragging the application into `Applications` succeeds;
- the application starts by double-clicking it;
- workspace settings, network confirmation, and live activity work in the startup window;
- the selected sandbox is created and opens correctly; and
- failure messages remain visible and explain how to recover.

Apply equivalent checks to every additional sandbox. If a sandbox needs a
different build command, document it in this file under its own heading.

## Publish with a GitHub Release

Publish the unified application as one repository-wide `v<version>` release
containing **VS-Code-Sandboxes.dmg** and **VS-Code-Sandboxes.exe**. These each
include all three sandbox types. Per-language packages are compatibility builds.

Update the unified application version before publishing. Rebuild and verify
both downloads with `./scripts/create-release.sh all macos` and
`./scripts/create-release.sh all windows`, then commit the source, metadata, and
artifacts together. Test Windows on a Windows host before publishing.

After the release commit is on the default branch, create an annotated version
tag and publish the two files under `sandbox-launcher/assets/` together with
`LICENSE` and `THIRD-PARTY-NOTICES.txt`. Include the tested operating systems and
checksums in the release notes. Verify that the uploaded downloads match the
local artifacts and mark the release as the latest stable release.

## License and dependency notices

Keep `LICENSE` and `THIRD-PARTY-NOTICES.txt` with every release, including releases
containing only one platform. Upload both documents as release assets and link
the notices in the release notes. Preserve existing notices when modifying or
redistributing third-party components.

macOS build scripts include both documents in the disk image and application.
Windows launchers embed them and expose them with `--licenses`; generated Windows
project runners retain the embedded documents. Generated macOS project runners
also carry the documents in their Resources directories.

When changing the Go toolchain, review its main license and component-specific
notices (including runtime and math sources) and update the third-party notices.
A permissive project license does not replace those notices.

The Python and R image recipes preserve runtime-stage package documentation and
an inventory of package and source-package versions under
`/usr/local/share/licenses/vscode-sandboxes/`. Some upstream images omit
documentation or contain dangling documentation symlinks. This archive preserves what is available; it does not certify complete
license coverage. Restore any missing notices before redistribution.
Preserve these directories and the final base image's license documentation. Built images are not release assets.
Before publishing an image, audit its complete, resolved dependency set and supply
any required corresponding source; a package inventory or upstream URL alone is
not sufficient for every license.

The Python icon is a derived trademark used for a freely distributed Python tool.
Follow the PSF trademark policy separately from the MIT license, especially for
commercial redistribution. Do not imply endorsement by upstream projects. Adding
official R logo artwork requires its own license and attribution; the current
letter-R artwork is not the official R-and-ellipse logo.


## Desktop startup controller

`internal/startui` contains the shared local startup window, authenticated
loopback controller, Codex TOML import, and network policy orchestration.
`internal/desktop/Window.swift` hosts the UI in a native macOS WebKit window.
Windows runs the same UI in an isolated Edge app window and starts hidden worker
processes using the existing Go launcher backend.

Run `./scripts/build-desktop.sh` on macOS to rebuild helpers in the three source
apps before opening them. DMG builders run this automatically. Generated helper
binaries are ignored by Git and are not embedded recursively into setup bundles.
The project setup copies the desktop helpers into generated macOS project apps.
Windows release builds use `-H windowsgui` to avoid opening a console.

Run `go test ./...`, `scripts/test-self-contained.sh`,
`scripts/test-daemon-autostart.sh`, `scripts/test-podman-workflow.sh`, and
`python3 scripts/test-apt-lock-retry.py`. Startup policy tests use a simulated
`sbx` and temporary user configuration directories, without changing live Docker
policies. Also compile Windows launchers and visually inspect the macOS window.
Test real network access, browser authentication, and Windows UI on their target
hosts before publishing releases.

The TOML parser is `github.com/pelletier/go-toml/v2` (MIT); its notice is included
in `THIRD-PARTY-NOTICES.txt`.


## Unified application releases

Use `./scripts/create-release.sh all macos` and
`./scripts/create-release.sh all windows` to update
`sandbox-launcher/assets/VS-Code-Sandboxes.dmg` and
`sandbox-launcher/assets/VS-Code-Sandboxes.exe`. These two files are the primary
user downloads. Existing per-language build targets remain compatibility builds.

The shared catalog in `internal/startui/catalog.go` drives the type selector and
platform dispatchers. Python/R request a language version and host Podman;
general bioinformatics requests neither. Fixed project runners resolve their
sandbox from their own configuration or filename and cannot switch an existing
workspace's type. The unified macOS package stages all worker commands, icons,
and self-contained setup bundles under `Resources/Launchers/`; these generated
payloads are ignored by Git and recreated by `scripts/build-desktop.sh`.

Build macOS packages with Go and Xcode command-line tools (`swiftc`), and inspect
all selector choices before publishing. Windows packages can be cross-compiled;
exercise the Edge app window on Windows before publishing.
