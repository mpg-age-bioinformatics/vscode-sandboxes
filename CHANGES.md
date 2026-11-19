# Changes

Detailed changes, validation results, testing limitations, and checksums for each release.

<a id="v1-1-2"></a>

## v1.1.2 — 2026-11-19

### Changes

- Give the macOS DMG a visual drag-to-Applications installation guide: the sandbox app on the left, an arrow in the middle, and the Applications folder on the right. Include brief instructions and keep license documents accessible below.
- Render the background at 1x and 2x resolution and save the Finder icon positions, background, and window settings before compressing the disk image.
- Keep GitHub release descriptions concise, with detailed changes and checksums in dated sections of this file. State missing Windows installation/end-to-end testing directly on the release page as well.

### Validation

Built on macOS 27.0.1. `go test ./...`, `go vet ./...`, all JavaScript UI regressions, packaging shell syntax checks, and compilation of the Finder layout AppleScript passed. The installation window was visually checked in Finder during preparation, including correction of the Retina background scaling. The final v1.1.2 disk image checksum, saved Finder metadata and background, application version, Applications shortcut, license notices, and 87 embedded setup files were verified. The Windows executable was cross-compiled and checked as an x86-64 PE executable with version 1.1.2 metadata.

A clean Windows 11 installation and end-to-end run have not been completed. Full interactive first-run testing was not repeated for this packaging update. R project setup and restart orchestration are covered by the fixture-based Go tests; a live R session and the dedicated R session regression were not run. The Python and shell workflow regression suites passed for v1.1.1 and were not repeated for this packaging-only update.

### SHA-256 checksums

```
36b4cff42aae223104ce6141e878c1720997c810b05a584e922a0bca58545795  VS-Code-Sandboxes.dmg
fd8ae6b36e7512eef9a32aed6fcdc3c25d94bed9deba225f5f231067373e57ee  VS-Code-Sandboxes.exe
b66258f4eeb4bb82f9da208d547c195d46f8535e96750d8720a43058e3c4cf0a  LICENSE
834ba0988f75c64008c47983a7cb55c0aaedc25ea56dafd2e1da37728f55ae66  THIRD-PARTY-NOTICES.txt
```

[Full changes](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/compare/v1.1.1...v1.1.2)

<a id="v1-1-1"></a>

## v1.1.1 — 2026-11-19

### Changes

- Show **Set up your computer** on every launch whenever Docker Sandboxes or Visual Studio Code is missing, including launches from project runners. Block workspace startup until both tools are detected; bring setup back if either tool is removed.
- Offer **Install missing components**, using official vendor downloads with pinned SHA-256 checksums, verified cache reuse, visible progress and errors, and automatic/manual detection refresh. Keep existing installations. The bootstrap downloads Docker Sandboxes 0.39.0 and VS Code 1.147.0 when missing; vendor binaries are not redistributed in these downloads.
- Install tools within the macOS user account, preserving the complete Docker runtime layout. Use the official Windows installers after verifying their Authenticode publisher signatures. Windows OpenSSH, virtualization setup, any administrator prompts/restarts, and Docker sign-in still apply.
- Remove the host Git and Git Bash requirements. The shared native launcher calls sbx directly and streams setup inputs into sandbox-local temporary storage. Skills cloning, repository initialization, setup commits, and Git history checks run inside the sandbox.
- Preserve staged edits and project history on restarts. Check saved setup ancestry inside the sandbox before reusing a completed environment.
- Update the README prerequisites to describe an OpenAI or Claude account, or a private LLM endpoint with its authentication key.

### Validation

Built on macOS 27.0.1. `go test ./...`, `go vet ./...`, all JavaScript UI tests, all Python regression scripts, all shell regression scripts, and shell syntax checks passed. Tests cover missing-tool combinations, project-runner setup gating, rechecks and removal, blocked startup after detection failures, checksum rejection, offline cache reuse, installer serialization/failures, and native startup with host Git/Bash/container tools blocked.

The setup screen was visually reviewed. A disposable live sandbox verified workspace mapping, sandbox Git, archive streaming, an initial commit, and a clean working tree with outbound access blocked; it was removed afterwards. The macOS DMG checksum and mounted version, setup UI, icon, notices, and embedded payloads were checked against source. The Windows build was verified as an x86-64 PE executable with version 1.1.1 metadata and the setup UI.

A clean Windows 11 installer/end-to-end run and full interactive first-run tests have not been completed for this release. Automatic vendor installation was tested with isolated fixtures, without replacing installed host software. The R session regression test was not run because Rscript was unavailable.

### SHA-256 checksums

```
f1d85812a4ad8b5d3241adbb0e29b7315971ffdaa990954b5439da768ff0757c  VS-Code-Sandboxes.dmg
62b7fda727abe71fce66996d1925a6610173db61963665e1814eb91be37eb7b5  VS-Code-Sandboxes.exe
b66258f4eeb4bb82f9da208d547c195d46f8535e96750d8720a43058e3c4cf0a  LICENSE
834ba0988f75c64008c47983a7cb55c0aaedc25ea56dafd2e1da37728f55ae66  THIRD-PARTY-NOTICES.txt
```

[Full changes](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/compare/v1.1.0...v1.1.1)

<a id="v1-1-0"></a>

## v1.1.0 — 2026-11-19

### Changes

- Unified startup settings, project folder selection, sandbox type/version selection, network controls, and live activity in one window, with a matching application icon.
- Recognize existing projects and offer Restart sandbox while preserving project files and staged edits. Export startup activity with API keys redacted.
- Run Python and R development containers inside a general Docker Sandbox, with persistent language environments, pinned image metadata, and cached environment reuse.
- Include both Codex and Claude Code. Use Codex with its default OpenAI connection, or supply a separate private endpoint URL and API key for Continue, with model discovery and sandbox-local configuration.
- Provide editable network allowlists for each sandbox type, automatically include private endpoint hosts, and reject private endpoints in offline mode.
- Require Python notebooks paired with synchronized Jupytext Python mirrors, pinned dependency manifests/locks, reproducible install commands, and an installation history in generated agent instructions.
- Improve R system-package tracking, coding-agent setup, and recovery of skills repositories without an origin while preserving their contents.

### Validation

Built and checked on macOS 27.0.1 (build 26A434). Go tests, all JavaScript UI regressions, all Python regression scripts, self-contained setup, daemon auto-start, Podman workflow, development-container host workflow, skills recovery, and shell syntax checks passed. The macOS DMG checksum was verified; its version, icon, notices, Applications shortcut, and 84 embedded setup files were checked against release source. The Windows build was checked as an x86-64 PE executable with 1.1.0 metadata and updated embedded instructions.

A clean Windows 11 end-to-end run and full interactive first-run checks were not repeated for this release. The R session regression test was not run because Rscript was unavailable on the build host.

### SHA-256 checksums

```
2e5b0a969473157933f598e9ccfbf1e0309cfbdbf5a617346f6bd71aae96d027  VS-Code-Sandboxes.dmg
ceb34704c24ccf6e51a15aca43fca2e21504f4ccff456316bb81efdd6236a61b  VS-Code-Sandboxes.exe
b66258f4eeb4bb82f9da208d547c195d46f8535e96750d8720a43058e3c4cf0a  LICENSE
834ba0988f75c64008c47983a7cb55c0aaedc25ea56dafd2e1da37728f55ae66  THIRD-PARTY-NOTICES.txt
```

[Full changes](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/compare/v1.0.2...v1.1.0)
