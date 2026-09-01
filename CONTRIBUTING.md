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
  --title "Python Sandbox 1.0.0" \
  --generate-notes
```

For an R Sandbox release, use the corresponding R-specific tag and artifact:

```bash
git tag -a r-sandbox-v1.0.0 -m "R Sandbox 1.0.0"
git push origin r-sandbox-v1.0.0
gh release create r-sandbox-v1.0.0 \
  r-sandbox/assets/R-Sandbox.dmg \
  --title "R Sandbox 1.0.0" \
  --generate-notes
```

For a Bioinformatics Sandbox release:

```bash
git tag -a bioinformatics-sandbox-v1.0.0 -m "Bioinformatics Sandbox 1.0.0"
git push origin bioinformatics-sandbox-v1.0.0
gh release create bioinformatics-sandbox-v1.0.0 \
  bioinformatics-sandbox/assets/Bioinformatics-Sandbox.dmg \
  --title "Bioinformatics Sandbox 1.0.0" \
  --generate-notes
```

Before publishing, confirm that the DMG in `assets/` is the same build that was
tested. The GitHub Release page—not a link to the `.app` directory in the source
tree—should be given to users. Users download the DMG, open it, and drag the app
to the provided `Applications` shortcut.

For another sandbox, replace the directory, tag, title, and artifact filename
with that sandbox's values. Publish one GitHub Release per sandbox version.

