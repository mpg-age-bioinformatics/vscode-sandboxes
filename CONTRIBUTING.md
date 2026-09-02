# Contributing

This repository keeps each sandbox and its downloadable release files in a
separate top-level directory. For example, the Python sandbox lives in
`python-sandbox/`.

## Repository convention

Each sandbox should contain everything needed to reproduce its user-facing
download:

```text
<sandbox>/
├── assets/     # Application bundles, icons, and release artifacts
└── scripts/    # Reproducible build and packaging scripts
```

Keep generated downloads in the sandbox's `assets/` directory and give them a
stable, descriptive name. A release must not depend on uncommitted files from a
developer's computer.

The macOS applications in this repository are bootstrap launchers. Their source
repository URL and revision behavior must be reviewed whenever the corresponding
sandbox setup repository changes.

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

Windows launchers are native x86-64 console applications built with Go. Install
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

The committed `windows/rsrc_windows_amd64.syso` files contain each executable's
icon, manifest, and version information. Regenerate them with the build scripts
whenever an icon or Windows metadata changes.

The initial Windows launchers must also verify that setup created the matching
`code/Run <Sandbox>.exe`. Those project runners are maintained in the corresponding
`skills/<sandbox>/assets/windows-project-runner/` directory and built with:

```bash
python-sandbox/scripts/build-windows-project-runners.sh
r-sandbox/scripts/build-windows-project-runners.sh
bioinformatics-sandbox/scripts/build-windows-project-runners.sh
```

Run those commands from the skills repository. Test both the Codex and Claude
runner variants. A clean generated project must contain the stable user-facing
filename, track it in Git, and reopen the existing sandbox without rerunning setup.

Test every executable on a clean, supported 64-bit Windows 11 system. Verify the
unsupported-Windows and disabled-hypervisor failures, prerequisite diagnostics,
interactive prompts, Git clone, project setup, sandbox connection, extension
installation, and VS Code opening. Python and R tests require a working Docker
engine; Bioinformatics Sandbox must work with `sbx` without Docker Desktop.

Then mount the DMG and verify that:

- the Finder window contains the application and an `Applications` shortcut;
- dragging the application into `Applications` succeeds;
- the application starts by double-clicking it;
- all interactive prompts work in a fresh Terminal session;
- the selected sandbox is created and opens correctly; and
- failure messages remain visible and explain how to recover.

Apply equivalent checks to every additional sandbox. If a sandbox needs a
different build command, document it in this file under its own heading.

## Publish with a GitHub Release

Use a sandbox-specific version tag so releases from different sandboxes cannot
collide. The convention is `<sandbox>-v<version>`, for example
`python-sandbox-v1.0.0`.

After the release commit is on the default branch:

```bash
git tag -a python-sandbox-v1.0.0 -m "Python Sandbox 1.0.0"
git push origin python-sandbox-v1.0.0
gh release create python-sandbox-v1.0.0 \
  python-sandbox/assets/Python-Sandbox.dmg \
  python-sandbox/assets/Python-Sandbox.exe \
  --title "Python Sandbox 1.0.0" \
  --generate-notes
```

For an R Sandbox release, use the corresponding R-specific tag and artifact:

```bash
git tag -a r-sandbox-v1.0.0 -m "R Sandbox 1.0.0"
git push origin r-sandbox-v1.0.0
gh release create r-sandbox-v1.0.0 \
  r-sandbox/assets/R-Sandbox.dmg \
  r-sandbox/assets/R-Sandbox.exe \
  --title "R Sandbox 1.0.0" \
  --generate-notes
```

For a Bioinformatics Sandbox release:

```bash
git tag -a bioinformatics-sandbox-v1.0.0 -m "Bioinformatics Sandbox 1.0.0"
git push origin bioinformatics-sandbox-v1.0.0
gh release create bioinformatics-sandbox-v1.0.0 \
  bioinformatics-sandbox/assets/Bioinformatics-Sandbox.dmg \
  bioinformatics-sandbox/assets/Bioinformatics-Sandbox.exe \
  --title "Bioinformatics Sandbox 1.0.0" \
  --generate-notes
```

Before publishing, confirm that the DMG and EXE files in `assets/` are the same
builds that were tested. Give users the GitHub Release page—not a link to the
`.app` directory in the source tree. macOS users install through the DMG; Windows
users download the matching EXE directly.

For another sandbox, replace the directory, tag, title, and artifact filename
with that sandbox's values. Publish one GitHub Release per sandbox version.
