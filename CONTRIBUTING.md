# Contributing

The primary application lives in `sandbox-launcher/` and includes Python, R, and
general bioinformatics in one startup UI. Per-language source directories remain
as worker and compatibility-build inputs. Shared setup payloads live under `internal/`.

## Reporting an issue

Use **Report an issue** at the lower right of the launcher, or open
[GitHub issues](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/issues).
Choose **Bug report** or **Feature request** and follow the form's guidance.
For bugs, include the launcher version, operating system/architecture, Local or
Remote workflow, reproduction steps, expected/actual behavior, and relevant
activity output. Include tool/cluster versions when known. Reports are public:
review logs and screenshots and remove credentials or sensitive information.

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
`code/.sandbox/Run <Sandbox>.exe` name and writes a compatibility marker (`both`) to a companion file.
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
installation, and VS Code opening. Python/R tests must run with host Docker and Podman unavailable. Verify the
standard shell sandbox's Docker Engine, image pull, Dev Containers CLI startup, and
VS Code's nested SSH/container connection. For general bioinformatics, verify Podman
build/run/exec inside each sandbox; no launcher requires Docker Desktop.

Then mount the DMG and verify that:

- the Finder window contains the application and an `Applications` shortcut;
- the unified DMG opens in icon view with the app on the left, Applications on
  the right, and a drag-to-install arrow and instructions between them;
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
`LICENSE`, `THIRD-PARTY-NOTICES.txt` and `DISCLAIMER.txt`. Keep the GitHub release description short:
a brief summary, download names, the versioned README link and unsigned/notarized
status, a link to the release's dated section in `CHANGES.md`, and license/notice
links. Put detailed changes, tested operating systems, validation results, testing
limitations, SHA-256 checksums, and the comparison link in `CHANGES.md` instead.
Also state missing Windows installation/end-to-end testing directly on the
release page whenever those checks have not been completed; do not hide that
limitation behind the detailed-changes link.
Include a short **macOS: Open an unsigned sandbox app** note, linking directly to
`README.md#open-an-unsigned-sandbox-app` at that release's full Git commit SHA
(`https://github.com/mpg-age-bioinformatics/vscode-sandboxes/blob/<release-commit>/README.md#open-an-unsigned-sandbox-app`).
Include a short **Use at your own risk** note linking to
`README.md#user-responsibility-and-liability` at that same release commit, and
link the attached `DISCLAIMER.txt`. Do not describe the disclaimer as absolute
immunity or remove its applicable-law qualification. Keep the MIT license intact.
Resolve the commit from the release tag with `git rev-parse <tag>^{}`; do not use
`main` or a newer README for an older release's installation instructions.
Maintain the **Release commits** table in `CHANGES.md` for every published
release. Resolve the full commit SHA with `git rev-parse '<tag>^{commit}'` and
link to `https://github.com/mpg-age-bioinformatics/vscode-sandboxes/commit/<sha>`.
Record this mapping in a documentation follow-up commit after the release commit
and tag exist: a commit cannot contain its own final SHA. Preserve published
tags and their release commits when updating the table.
Use a version-and-date heading and a stable version anchor for each entry. For
existing tags that predate `CHANGES.md`, link to the documentation commit containing
that entry; never move a published tag to add the file. Verify that uploaded
downloads match local artifacts and mark the release as the latest stable release.

## License and dependency notices

Keep `LICENSE`, `THIRD-PARTY-NOTICES.txt` and `DISCLAIMER.txt` with every release, including releases
containing only one platform. Upload all three documents as release assets and link
the notices in the release notes. Preserve existing notices when modifying or
redistributing third-party components.

macOS build scripts include all three documents in the disk image and application.
The unified DMG builder renders its installation background from
`internal/desktop/DMGBackground.swift` at 1x and 2x resolution and saves the
Finder window layout with `sandbox-launcher/scripts/layout-dmg.applescript`
before compressing the final image. Its hidden background and `.DS_Store` are
packaging metadata and must survive conversion to the read-only image.
Windows launchers embed all three and expose them with `--licenses`; generated Windows
project runners retain the embedded documents. Generated macOS project runners
also carry the documents in their Resources directories.

When changing the Go toolchain, review its main license and component-specific
notices (including runtime and math sources) and update the third-party notices.
A permissive project license does not replace those notices.

Python and R now pull complete upstream development images inside the sandbox;
no runtime binaries are copied between images. Record the resolved image digest
and preserve the images' original notices. The language images and downloaded
agent/Dev Containers tooling are not bundled as release assets. Before publishing
custom images, audit their dependencies and corresponding license requirements.

The Python icon is a derived trademark used for a freely distributed Python tool.
Follow the PSF trademark policy separately from the MIT license, especially for
commercial redistribution. Do not imply endorsement by upstream projects. Adding
official R logo artwork requires its own license and attribution; the current
letter-R artwork is not the official R-and-ellipse logo.


## Desktop startup controller

`internal/startui` contains the shared local startup window, authenticated
loopback controller, private Continue connection setup, and network policy orchestration.
`internal/desktop/Window.swift` hosts the UI in a native macOS WebKit window.
Windows runs the same UI in an isolated Edge app window and starts hidden worker
processes using the existing Go launcher backend.

Run `./scripts/build-desktop.sh` on macOS to rebuild helpers in the three source
apps before opening them. DMG builders run this automatically. Generated helper
binaries are ignored by Git and are not embedded recursively into setup bundles.
The project setup copies the desktop helpers into generated macOS project apps.
Windows release builds use `-H windowsgui` to avoid opening a console.

The unified app uses the startup UI's white striped square on a teal tile as its
application icon. On macOS, run `bash scripts/build-app-icon.sh` to regenerate
`sandbox-launcher/assets/SandboxesIcon-1024.png` and the app's `Sandboxes.icns`
from `scripts/render-app-icon.swift`. The Windows build script embeds the same
PNG in its executable resources. Rebuild both releases after changing the icon.

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
platform dispatchers. Python/R request a language version and start a standard
Docker-enabled shell sandbox; no selection requires a host container engine. Fixed project runners resolve their
sandbox from their own configuration or filename and cannot switch an existing
workspace's type. The unified macOS package stages all worker commands, icons,
and self-contained setup bundles under `Resources/Launchers/`; these generated
payloads are ignored by Git and recreated by `scripts/build-desktop.sh`.

Build macOS packages with Go and Xcode command-line tools (`swiftc`), and inspect
all selector choices before publishing. Windows packages can be cross-compiled;
exercise the Edge app window on Windows before publishing.


## Language containers inside a general sandbox

The shared implementation is under `internal/launcherbundle/bundles/common/assets/`.
`Materialize` merges these assets into Python/R bundles, and macOS packaging stages
them alongside each language's assets. Direct source setup resolves the same
shared directory. Keep all three paths covered by self-contained payload tests.

`run-language-sandbox.sh` runs on the host and invokes only sbx, SSH, and VS Code.
`start-development-container.sh` runs inside the sandbox and uses its Docker
Engine plus pinned Dev Containers CLI. Image-based `code/.sandbox/devcontainer.json`
configurations replace the old multi-stage sandbox-image recipes. Development
containers get their own non-root user and volumes for home and language packages;
the project remains host-backed. Record the base-image digest and Dev Containers
feature lockfile in code. No Docker socket is mounted into the development container.

Run `bash scripts/test-development-container-workflow.sh` to reject host Docker or
Podman calls and verify policy failures stop before container startup. Run
`python3 scripts/test-development-container-config.py` for digest pinning, proxy and
credential mount configuration, image-cache reuse, and nested VS Code URI encoding.
The Go bundle tests verify the shared assets are embedded on Windows. Existing
bioinformatics Podman/apt checks remain applicable only to that sandbox type.
Run `python3 scripts/test-r-system-packages.py` to check that cached R system
libraries require no package-network access and incompatible locks fail clearly.

For a real integration test, create a disposable shell sandbox with a temporary
workspace. Verify both language containers, their runtime versions, both agent CLIs,
private-config mounts, offline cache reuse, and the selected network policy.
Remove only the named disposable sandbox afterwards. The general bioinformatics
workflow and existing user sandboxes must remain unaffected.

Run `python3 scripts/test-coding-agents.py` to verify that both CLIs are installed,
remain on terminal PATH, and reopen without reinstalling cached packages.

Run `node scripts/test-site-presets.js` for language-specific site switching,
custom edits, saved-state migration, and restoring defaults. Preset host changes
should also pass `go test ./internal/startui` and be checked against vendor docs.

Run `node scripts/test-connection-ui.js` for automatic OpenAI/private connection
selection, paired URL/key validation, startup without connection files, and masked key retention after startup. Run
`node scripts/test-endpoint-network-ui.js` for automatic endpoint allowlisting and
offline validation. Keys must never appear in persistent launcher preferences or
project files. The Go model-discovery tests use local HTTP servers to verify
Bearer headers, URL paths, response limits, and redirects without sending
credentials to external services.

The local Continue execution setting uses `github.com/tailscale/hujson` (BSD-3-Clause)
to preserve JSONC comments and unrelated VS Code settings. Its tests cover existing
overrides, invalid files, and idempotent startup.

Project reuse and initial Git baselines are covered by the project-state Go tests.
These verify one clean initial commit, preservation of staged user edits on
restarts, restoration of language/version, and rejection of stale success records.
Run `node scripts/test-activity-export-ui.js` for native/browser export behavior.
The development-container config tests also verify that resumed containers skip
initialization but still run environment checks.

## Native host startup

`internal/startui/native_launch.go` is the shared macOS/Windows dispatcher. It
calls the native `sbx` CLI, streams embedded setup files to sandbox-local temporary
storage, and runs setup, skills cloning, repository checks, and commits inside
the mounted sandbox workspace. Host Git, Git Bash, cygpath, tar, Python, Node, and
container engines are not runtime prerequisites. Windows launches VS Code's CLI
through `Code.exe` with `ELECTRON_RUN_AS_NODE=1`, without a command shell.

Project selection reads local metadata without invoking Git. After mounting the
workspace, startup validates the saved setup commit inside the sandbox before
resuming initialization. Repository and folder identities prevent stale success
records from silently reusing replaced projects. Existing per-project launchers
keep their generated configuration; upgrading their bundled controller is needed
to use the native path.

Run `go test ./...` for native startup regressions. The boundary tests execute
real setup/Git against temporary fixtures behind an emulated sbx CLI while host
Git, Bash, cygpath, Docker, and Podman are traps. They cover all three fresh setup
flows, a single initial commit, staged edits on restart, and policy/runtime
failures. The legacy shell workflow tests remain for compatibility entry points.

## Prerequisite setup screen

`internal/startui/prerequisites.go` detects Docker Sandboxes and VS Code on every
launch and discovers newly installed tools outside the process's original PATH.
The shared UI gates both first-time and project startup, refreshes detection, and
only installs after an explicit Install missing components action. The controller
rechecks readiness before startup, serializes installation with startup, and
keeps the window open while installing.

Vendor downloads are pinned by version and SHA-256. Updating those constants
requires checking the official Docker release digests or Microsoft update API
metadata, then testing the installers on each target OS. macOS uses the complete
Docker archive layout and Microsoft's signed app; Windows verifies Authenticode
publisher signatures before invoking the official installers. Existing installs
are never overwritten. Vendor binaries belong in the user's download cache,
not the repository or release assets.

Run `node scripts/test-prerequisites-ui.js` and `go test ./internal/startui` for
missing-tool combinations, project-launcher gating, install/recheck transitions,
removal detection, verified downloads, offline cache reuse, checksum rejection,
installer serialization/failures, and preservation of existing installs.

### Remote HPC launcher

Run `go test ./internal/startui -run TestRemote` and
`node scripts/test-remote-ui.js`. Scheduler fixtures exercise submission arguments,
compute-node mounts, status, interrupted-submission recovery and identity-checked
cancellation without contacting a cluster. The launcher test verifies the
container endpoint before invoking a fake VS Code CLI, reconnects without another
submission, and checks SSH settings and cancellation persistence.

For site validation, use an approved SIF and disposable persistent project. Verify
queue wait, a real Remote-SSH connection through the login node, Python/R/notebook
execution, Codex sign-in and execution inside the container, reconnect, time-limit
expiry, and cancellation. Record the cluster/site policy and results; fixture
passes do not establish that an HPC permits the service.

Remote image source tests also verify shared Local preset assets, self-contained
Dockerfile uploads, sandbox-only local build orchestration, target architecture,
image upload staging, compute-node conversion, project preservation and version
conflicts. The UI tests exercise all source choices and conditional tool setup.
For live validation, additionally test an amd64 image build on Apple silicon,
archive transfer, compute-node SIF conversion and first-time Python/R package
initialization. Retain resulting image locks and recipes with the project.

To opt into an actual local image build (without SSH or Slurm), run:

```bash
VSCODE_REMOTE_LIVE_BUILD=1 go test ./internal/startui -run '^TestRemoteLiveImageBuild$' -v -timeout 30m
VSCODE_REMOTE_LIVE_BUILD=1 VSCODE_REMOTE_LIVE_PRESET=r go test ./internal/startui -run '^TestRemoteLiveImageBuild$' -v -timeout 30m
```

These default to amd64 Python 3.13 or R 4.5. `VSCODE_REMOTE_LIVE_ARCH` and
`VSCODE_REMOTE_LIVE_VERSION` override those choices. The test retains the reusable
Dockerfile and archive in the application cache and removes its temporary sandbox.

The R preset's Dockerfile now repairs only affected libraries in the R runtime
dependency graph for 16 KiB page compatibility. `TestRemoteRAlignmentScopeAndAtomicFailure`
covers ELF selection, compatible/native skips, dependency scope and atomic failure
preservation. The repair verifies executable-section contents/addresses, SONAME
and dependency names before replacement. New R image builds must execute
`smoke-r.R` successfully (ICU collation, CSV round trip, PDF and Cairo PNG); image
installation alone is not sufficient validation.

Remote input regressions cover tilde expansion, comma/whitespace normalization,
multiple quoted folder paths, named module loading in login/status/compute shells,
legacy preference migration, and default Slurm accounting even when
`SBATCH_ACCOUNT` was set. These changes do not require rebuilding the image cache.

To test against an HPC you are authorized to use (real upload and Slurm allocation):

```bash
VSCODE_REMOTE_LIVE_HPC=your-ssh-host go test ./internal/startui -run '^TestRemoteLiveHPC$' -v -timeout 30m
VSCODE_REMOTE_LIVE_HPC=your-ssh-host VSCODE_REMOTE_LIVE_PRESET=r go test ./internal/startui -run '^TestRemoteLiveHPC$' -v -timeout 30m
```

This uses the default SSH identity, partition `cluster`, one CPU and 8 GiB, with
an abbreviated 20-minute allocation. It creates separate smoke-test projects under
`~/sandboxes`, verifies additional-folder mounts, container SSH, runtime setup,
and a fresh SSH reconnect, then cancels only its own allocation. Project files,
logs and uploaded images remain for inspection; per-allocation local SSH keys and
aliases are removed. It does not launch VS Code or authenticate a coding agent.

For a startup-only retry against a SIF created by a previous live test, set
`VSCODE_REMOTE_LIVE_STAGE` to that test's remote upload directory and
`VSCODE_REMOTE_LIVE_CACHE` to its local image cache directory. This uploads the
current project template into that test stage and creates a fresh project/job,
without uploading or converting the image again. Use only a previous stage you
own, with a recipe matching the selected preset/version/architecture.

`VSCODE_REMOTE_LIVE_PROJECT` can select the previous smoke-test project to verify
a startup retry with its existing library and dependency locks. The live test also
checks the effective container identity environment; the local OpenSSH-server
regression validates effective `SetEnv` configuration with `sshd -T`.

Remote transport must never SSH or forward TCP to a compute-node address.
`ProxyCommand` authenticates to the login node and runs a token/job-checked
`srun --jobid` relay to the container's loopback endpoint. Slurm unbuffered mode
uses a PTY, which the Python relay switches to raw mode before sending bytes.
Test all byte values, concurrent connections on a one-CPU allocation, modern
`--overlap` and older Slurm, and absence of direct-compute fallbacks.
`VSCODE_REMOTE_LIVE_CLUSTER_TMP=1` exercises a site-defined temporary path with
spaces. Assert that temporary storage stays under that path, that container SSH
inherits TMPDIR, and that R can use the space-free container alias.

Server image metadata is implemented by `remote_assets/catalog.py`, invoked over
login-node SSH. Tests cover incomplete/missing images, recipe lookup, image identity,
project-only transfer on cache hit, concurrent conversion, metadata ownership,
allocation discovery, public-key registration and identity-checked stop. Keep
metadata size bounded and atomic; never infer running jobs solely from stale files.
Image recipes/builder inputs determine the local cache key; project instructions
and runtime initialization are regenerated separately.

For a live cache test using a previously validated SIF on the same HPC filesystem,
set `VSCODE_REMOTE_LIVE_CACHE_SIF` to its remote path and
`VSCODE_REMOTE_LIVE_CACHE` to the matching local cache directory. The test checks
that the recipe still matches, registers the existing SIF with a hard link, and
exercises the production cache lookup/project transfer. `VSCODE_REMOTE_LIVE_CACHED=1`
selects the image explicitly from the catalog instead of using automatic recipe
lookup. It also discovers the new job, registers a different laptop key, verifies
reconnect/concurrent/binary transport, and stops the selected allocation.
