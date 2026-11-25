# Changes

Detailed changes, validation results, testing limitations, and checksums for each release.

## Release commits

Each commit link identifies the exact commit referenced by the published release tag.
Add an entry for Unreleased changes when their release is tagged.

| Release | Commit |
| --- | --- |
| [v1.2.1](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/releases/tag/v1.2.1) | [1c3da81313ba](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/commit/1c3da81313ba0c2efe059e083c1d480e08d7f2f3) |
| [v1.2.0](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/releases/tag/v1.2.0) | [8d0feec1f682](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/commit/8d0feec1f682d54fce49fd8d2c74774037345112) |
| [v1.1.2](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/releases/tag/v1.1.2) | [0bb8607cef8b](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/commit/0bb8607cef8b64200cf376705534e4b124257105) |
| [v1.1.1](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/releases/tag/v1.1.1) | [0aa30746c6eb](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/commit/0aa30746c6eb8e3ebb37de5a64f43ca864f96d19) |
| [v1.1.0](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/releases/tag/v1.1.0) | [6e14e40f191e](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/commit/6e14e40f191ef3544c684a65251f24309b304a2a) |
| [v1.0.2](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/releases/tag/v1.0.2) | [caf41d1aa85c](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/commit/caf41d1aa85c6d27e849e65de9e8f8043fe01b86) |
| [v1.0.1](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/releases/tag/v1.0.1) | [93639d49739d](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/commit/93639d49739daf3c60691023432d74f1765d95c6) |
| [v1.0.0](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/releases/tag/v1.0.0) | [2cbdb003765f](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/commit/2cbdb003765f260238c04935a0581f6cd16f0c5d) |
| [v0.0.1](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/releases/tag/v0.0.1) | [414357ae1bc4](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/commit/414357ae1bc422fe20ad2151e278b4d3c8085723) |

## Unreleased

- Open local and remote sandbox editor sessions with Workspace Trust disabled,
  preventing VS Code Restricted Mode from disabling extensions in new sandboxes.
  This applies to the launched session without changing global user settings.

- Show the build's Git tag/revision at the lower left of every launcher screen.
  Stamp macOS and Windows binaries at build time, include the development/dirty
  suffix, and retain a development fallback without a runtime Git dependency.

- Disable Codex's hosted Apps integration in Selected sites/No internet in
  addition to built-in web search. A new live conversation used
  `mcp__codex_apps__search_service_web_run` after built-in search was disabled.
  Set `features.apps=false` and disable default app/tool/open-world access;
  preserve original values for Open-mode restore. An installed-extension
  app-server check verified disabled Apps and no `codex_apps` MCP inventory.
  A live read-only test using the installed extension backend returned
  `HOSTED_TOOL_UNAVAILABLE` with no hosted web events. Current Local settings
  were updated; the live Remote refresh timed out and needs a reachable server.

- Disable Codex hosted web access in Selected sites as well as No internet.
  A live extension trace showed Code Mode opening CNN/SAPO URLs outside the
  configured search domains. Domain-filter serialization alone was not a
  sufficient enforcement test. Set both `web_search="disabled"` and
  `tools.web_search=false`, preserve original settings for Open-mode restore,
  and direct URL retrieval through sandbox commands. Configuration regressions
  and a mock Responses capture verify that new CLI requests omit hosted search.

- Apply agent web-tool policy from Selected sites for both Local and Remote,
  including existing project restarts and allocation reconnects. Configure
  Codex domain filters, Claude search/fetch hooks, supported Chat domain
  settings, and Continue revision-3 hosted-search blocking. Preserve endpoint
  credentials, unrelated settings/hooks and approval prompts; restore managed
  keys on Open. Document domain/subdomain differences, omitted port/IP rules,
  provider/MCP limits and the user-writable configuration boundary in APPTAINER.md.
  Validation: Go suite/vet, Python configuration/Claude-hook transitions,
  Continue's shared-search guard and UI regressions passed. Codex 0.163.0 sent
  the expected domain filters to a local mock Responses endpoint. Live Local
  and Remote settings/package checks passed. Interactive hosted searches for
  every provider and native Windows UI behavior were not exercised.

- Include exact Codex/ChatGPT authentication and backend hosts in the visible
  Selected sites defaults for every Local language preset. Existing edited
  lists stay under user control; Restore default sites includes the corrected
  entries. Docker's blocked-host log confirmed auth.openai.com, chatgpt.com,
  chat.openai.com and ab.chatgpt.com were missing from the previous defaults.

- Verify Docker's implicit default-deny decision with its read-only policy
  authorizer when no named default rule appears in the listing. This fixes
  Selected sites startup on Docker Sandboxes 0.39 without resetting an already
  restrictive shared policy. Reject allowed, explicit-deny, sandbox-scoped and
  malformed verification responses before removing the startup guard.

- Bind an explicit read-only loopback hosts file into isolated Remote
  containers. This fixes ChatGPT/Codex login callback forwarding failing to
  resolve `localhost` under Selected sites, without enabling external DNS or
  changing the egress allowlist. Apply the same fix to No internet loopback.

- Selecting Remote → Start a new sandbox now releases the launcher's current
  allocation selection and unlocks the form. The existing Slurm job stays
  running and remains available under Connect to running sandboxes. An ongoing
  startup still locks its settings until it completes.

- Fix the SSH authentication callback consuming scripted container commands on
  fresh connections. Use `ssh -n`, keep callback output off the protocol stdout,
  and retain exact container-proof validation. Distinguish SSH transport errors,
  empty successful responses and unexpected output instead of displaying `<nil>`.
  Reproduced the empty response on a fresh connection to the existing allocation;
  verified the complete container/job proof after correcting its managed hook.

- Recover bounded shared-folder directory-creation races during Local setup.
  Check existing directories first, retry only EEXIST-like creation errors,
  preserve conflicting files, and retry the sandbox setup RPC once for the exact
  `mkdir: Already exists` bridge failure. Recreate tar stdin on retry and keep
  permission, Git and other setup failures visible. Apply the directory helper
  to Python, R and bioinformatics project setup.
  Directory and bridge-error regressions, self-contained setup, skills recovery,
  the Go suite and vet passed. The partial Python project launcher was repaired
  inside its existing sandbox; its live startup completed with Python 3.13.18,
  Continue, Codex/Claude and the Codex command-sandbox check verified.

- Harden trusted Python control scripts with isolated imports, blocking project
  module shadowing before Apptainer starts and during host control operations.
- Deliver desktop session capabilities over inherited pipes; use nonpersistent
  macOS web storage and exact-origin navigation, and an ACL-protected temporary
  bootstrap with Edge InPrivate on Windows. Unix askpass uses a private Unix
  socket instead of exposing a cross-user reachable TCP authentication bridge.
- Require HTTPS for private model endpoints except loopback, preserve Continue's
  upstream permission prompts (distribution revision 2), and reject non-public
  proxy destination IPs unless explicitly enabled for internal cluster access.
- Anchor each new allocation's SSH server identity from the host batch process
  outside container-writable metadata. Validate SSH executables and parent chains,
  ignore writable heartbeats for anchored jobs, and reject future/non-finite
  timestamps. Existing allocations need recreation. Short sessions are recorded
  by the SSH client through an authenticated host login outside writable runtime
  files. Retain fail-safe behavior on uncertain liveness.
- Add security regression and vulnerability gates for pushes/pull requests,
  with action commit pins and read-only repository permissions.
- Build with Go 1.27.4 and update the retained toolchain notices. The patched
  source vulnerability scan reports no vulnerabilities. Native Windows behavior
  and real Raven password/2FA completion still require separate interactive tests.

Security validation: the full Go suite, vet, UI regressions, targeted race checks,
14 watchdog regressions, 7 network regressions, Continue distribution tests and
editor provisioning fixtures passed. Source and rebuilt Windows-binary scans
report no vulnerabilities. The native macOS app starts with no API capability in
its process arguments; the installed user app was updated and verified. Rebuilt
macOS/Windows packages include the patched toolchain and matching notices.

Live checks on hpc.bioinformatics.studio passed for two ordinary allocations and
for Selected sites/No internet allocations, using an existing SIF without a new
image upload/build. Host-written server identity and client-authentication event
checks passed. Restricted proxy use, blocked direct egress, offline isolation and
extension registration passed. The tests cancelled only their own allocations.
Native Windows operation, the GitHub workflow on hosted runners and completed
Raven password/2FA authentication are not claimed by these checks.

<a id="v1-2-1"></a>

## v1.2.1 — 2026-11-22

- Automatically show in-app SSH authentication popups for password, key
  passphrase and multi-step 2FA requests from image catalogs, uploads and remote
  job operations. Pass single-use responses through a temporary, capability-
  protected loopback askpass bridge; never persist them or place them in process
  arguments, environments or activity logs. Clear inputs on submission, cancel
  and expiry. Keep strict host verification and disable agent forwarding.
  Reuse authenticated OpenSSH connections on macOS/Linux for the VS Code relay.
  Remove the Terminal sign-in script; keep an optional Test SSH connection action.
  Verified that Raven's real password challenge opens in the macOS launcher.
  Completing Raven's authenticated password/2FA exchange remains untested.

- Show local Remote image-build cache disk usage at the bottom of both Remote
  views, with Clear cache and automatic refresh after deployment or cleanup.
  Clear only managed local build files; preserve HPC images/allocations, projects,
  SSH credentials and shared Docker template storage. Use cross-process locks
  to protect active builds/uploads, reject linked cache roots and avoid following
  nested symlinks. Cover cleanup scope, disk allocation, concurrent use, process
  crashes, HTTP guards and UI feedback with regressions.
  Go/vet and UI checks passed on macOS; both installers rebuilt. Windows cache
  locking and storage reporting were cross-compiled, without a native Windows run.

- Add README screenshots of Local setup, private endpoints, Remote creation and
  allocation connection, plus an overview figure at the beginning of the README.

### Release validation

The complete Go suite, Go vet and JavaScript UI regressions passed on macOS.
The SSH helper process fixture passed password followed by 2FA, cancellation,
bridge closure, single-use responses and no credential persistence; the askpass
regressions also passed with Go's race detector. The actual Raven password popup
was visually verified in the native launcher, without entering credentials.
The cache cleanup and concurrency tests cover the local image-build cache;
no user cache or HPC allocation was deleted for this release.

Both downloads were rebuilt with 1.2.1 version metadata. The macOS DMG checksum,
version, application layout, notices and embedded launcher were verified.
The Windows executable was cross-compiled on macOS. A clean Windows 11
installation/end-to-end run and native Windows password/2FA have not been tested.
Full authenticated Raven login, interactive VS Code/notebook first-run checks and
the dedicated graphical R session regression were not repeated for this release.
The live R runtime/ICU tests from v1.2.0 remain documented in that release section.

### SHA-256 checksums

```
a4287404cdfb2ba7e7bb5cfe0aeac7ef8862fcf3bcfe843933b6d84a14ea7542  VS-Code-Sandboxes.dmg
670503ac5cb657212b75182643c2846d204d51d77d278b222d2c92c8d2869412  VS-Code-Sandboxes.exe
b66258f4eeb4bb82f9da208d547c195d46f8535e96750d8720a43058e3c4cf0a  LICENSE
6d3c58cbf8092ffce8589b92d8a5e71ec3ff9b03f609bdba6f3b81b68fb2222c  THIRD-PARTY-NOTICES.txt
2f704641e01eaa72ef7d14bf8bb55fe895d97426e224d69e53dd4d77f69ec6b6  DISCLAIMER.txt
```

[Full changes](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/compare/v1.2.0...v1.2.1)

<a id="v1-2-0"></a>

## v1.2.0 — 2026-11-21

### Changes

- Add Remote No internet / Selected sites / Open choices. Restricted modes use
  rootless Apptainer network namespaces, a read-only UNIX-socket allowlist
  gateway outside the workspace, and Slurm/UNIX-socket SSH transport. Verify
  isolation before project setup and fail closed; old allocations remain Open.
- Provision the exact VS Code Server and default extensions separately without
  project/data/credentials mounted. Offline workspace setup skips online package
  resolution and skills cloning. Document the implementation, vendor bootstrap
  traffic and authority/port filtering limitations in APPTAINER.md.
- Register the pinned Continue distribution in the preloaded editor's actual
  extension inventory, then verify it with `code-server --list-extensions`.
  Stream large launch arguments over SSH stdin to avoid OS command-line limits.
- Validation: the complete Go suite and UI/gateway/editor fixtures pass. Live
  unprivileged Apptainer 1.2.5 checks on hpc.bioinformatics.studio passed with a
  Python SIF in Selected sites and an R SIF in No internet: permitted HTTPS,
  denied HTTP(S), direct IPv4/IPv6 and proxy bypass, Node fetch, exact binary SSH bytes,
  registered Continue, server CLI, R computation/file output, read-only mounts,
  sanitized environment, private PID/IPC namespaces, capabilities and seccomp.
  The tests used disposable jobs and retained the cached SIFs. Authenticated
  model calls, full interactive VS Code first-run and Windows end-to-end use
  remain untested; the Windows installer was cross-built only.

- Compact VS Code's duplicated virtual-environment prompt from `((.venv) )`
  to `((.venv))` in Remote interactive Bash. Preserve activation, custom prompt
  hooks and command exit status; leave ordinary prompts unchanged.

- Report Remote archive upload percentage, bytes, average speed and ETA from
  the existing SSH stream, throttled to roughly two-second activity updates.
  No extra SSH connections, remote polling or network probe transfers are used;
  distinguish streamed bytes from completed remote SHA-256 verification.
- Install the verified pinned Continue distribution inside every new remote
  container before SSH/VS Code starts, including cached/custom SIF launches.
  Correct Remote SSH defaults to use a valid extension ID instead of the
  unsupported version-qualified ID. Repair the current remote installation.
  Live Python and R tests verified the pinned extension before VS Code Server
  existed. Stream tests verified unchanged bytes, one SSH connection, throttled
  progress and separate success/failure reporting.

- Prevent new Remote image builds when the host has less than 20 GiB free;
  check the builder/workspace too and preserve a 2 GiB host reserve while
  building. Report Docker storage I/O/quota failures as storage problems.
- Resolve immutable registry descriptors without pre-pulling/extracting base
  images into the daemon. Export BuildKit directly to a validated Docker archive
  instead of loading another daemon copy and then saving it. Remove partial
  archives and clean up temporary builders on failure. Existing verified caches
  remain usable without the fresh-build free-space requirement.

- Add **Export activity…** to Remote, including startup output, status/errors
  and local tool setup activity. Use the native save dialog on macOS and a
  browser download elsewhere, defaulting to `vscode-sandox.yyyymmdd.hhmmss.log`.

- Harden Remote Apptainer launches using MPCDF's strict no-ptrace seccomp profile,
  private PID/IPC namespaces, zero capabilities, verified NoNewPrivs, suppressed
  automatic host binds and a restricted launch environment. Reject home,
  credentials, startup, runtime and launcher-control paths as workspace mounts.
- Mount launcher controls read-only with a separate writable runtime directory.
  Adapt the singleton watchdog to resolve the exact SSH process across private
  PID namespaces using namespace identity, NSpid and process start ticks.
- Carry policy assets, the technical APPTAINER.md guide, MIT attribution and
  Slurm explanation stubs in generated image recipes. Kernel restrictions are
  enforced at launch, including when reusing an existing cached SIF.
- Live CPU tests with cached Python/R SIFs on Apptainer 1.2.5 verified hidden
  real-home markers, ignored inherited bind/env injection, read-only control and
  additional-folder mounts, writable project/fake home, private PID/IPC
  namespaces, zero capabilities, NoNewPrivs, seccomp denials, Python fork/threads,
  Node workers, Codex/Claude CLI startup and an R runtime smoke check. GPU,
  authenticated model execution, and interactive editor testing remain untested
  for this strict policy. Image asset installation was executed in an isolated
  fixture; a full new image build was not repeated for this policy change.

- Add one server-side watchdog per HPC user for all new Remote allocations.
  Track authenticated connections to each allocation separately, cancel after
  24 disconnected hours, and exit automatically when no allocations remain.
  Never renew another sandbox through UI polling, inventory or a connection to
  a different sandbox. Verify allocation identity and recheck connections before
  cancellation; retain allocations when connection state cannot be verified.
- Default new Remote profiles to a 48-hour Slurm limit so the disconnected
  timeout can take effect. Preserve saved limits and cluster restrictions.
- Test concurrent registration, independent clocks, authenticated process
  ancestry, PID reuse, reconnect races, probe failures and automatic exit.
  All 12 watchdog regressions passed on macOS and the HPC's Linux/Python 3.8.
  Live HPC tests passed on 2026-11-20 across three pairs of allocations
  (Python/Python, Python/R and R/Python):
  verify one watchdog and its lock, retain the connected
  sandbox, cancel the disconnected sandbox, then cancel the remaining sandbox
  after disconnection and verify the watchdog process exits and releases its
  lock. The test accelerates only
  disposable allocations' clocks, rather than waiting 24 hours or changing the
  production timeout.

- Rename the application to **VS Code - Sandboxes** in the UI, documentation,
  macOS app/DMG names and Windows product metadata.

- Add a persistent **Report an issue** link in the lower-right corner of the
  launcher, and GitHub bug-report/feature-request forms with reporting guidance.

- Add Remote private endpoint URL/key fields and the same Continue/Codex choice
  as Local. Discover models from inside the HPC container; apply credentials at
  submission or attachment/reconnect before VS Code opens, without a separate
  Apply step. Keep keys out of saved profiles/catalogs/logs and
  write owner-only agent configuration in allocation-local home via SSH stdin.
- Validate Remote/Local configuration parity, credential redaction, refusal of
  authenticated cross-origin redirects, restoration, and UI credential routing.

- Include Bash and bash-completion in new Remote images, enable Tab completion
  in allocation shell startup files, and use Remote SSH's automatic remote
  extension installation for pinned Continue instead of the desktop-only CLI.

- Default Remote terminals to an allocation-local Bash profile, so Python's
  `source .venv/bin/activate` works even when the HPC account shell is /bin/sh.
  Keep explicit remote terminal preferences and use the opened project as cwd.

- Open Remote projects at their exact expanded HPC path with read/write mounts;
  mount additional folders read-only and reject overlaps with the project.
  Container home and editor/agent state are allocation-local; clarify persistent
  project storage, HPC permissions and separate launcher/image-cache records.
  Existing allocations retain their original mounts until replaced.
  Live Python 3.13.18 and R 4.5.3 checks passed for exact paths, project writes,
  read-only additional folders, reconnect and files surviving cancellation.
  Python command entry points were verified after legacy-path migration; R ICU,
  CSV, PDF and PNG checks passed.
- Separate Remote R startup dependency checks from the graphical regression test.
  A live HTTPGD device-close smoke test hung on the test HPC; it is not counted
  as passed. Keep the full graphical check available in verify-renv.R, and use
  a package/version/library check to gate startup instead.

- Show container/project preparation phases and stream Slurm startup logs while
  a running allocation prepares its packages, instead of reporting it as queued.

- Split Remote into **Start a new sandbox** and **Connect to running sandboxes**
  subtabs, with shared connection settings and automatic allocation discovery.
  Keep current-allocation controls inside the new-sandbox tab, visible only while
  an allocation is active; Connect uses its selected allocation’s Attach/Stop actions.

- Make user responsibility, absence of warranty and limitation of liability
  visible in the startup UI and README. Add a canonical `DISCLAIMER.txt`, qualified
  by applicable law, without changing the MIT License. Include it in macOS DMGs,
  app resources, Windows executable notices and generated Local/Remote projects;
  require it as a release attachment with a short version-specific release link.

- Keep the existing startup workflow under **Local** and add a **Remote** tab for
  Slurm + Apptainer workspaces. All cluster paths, SSH login, resources and module
  commands are configurable. Remote requires OpenSSH and VS Code on the laptop,
  with local Docker Sandboxes required only when building a new image.
- Submit and monitor one allocation, run SSH and VS Code Server inside Apptainer,
  verify that endpoint before opening the workspace, and install Codex/Python/R/
  Jupyter extensions remotely. Mount the writable project at its exact HPC path, additional folders read-only,
  and keep container home allocation-local. Include an adaptable image recipe with Python and R notebook kernels.
- Save profiles and allocation identity, recover interrupted submissions, reconnect
  without submitting again, and cancel only the identified live Slurm job. Use
  per-allocation SSH keys, pinned container host keys and no agent forwarding.
- Offer Python/R/general bioinformatics presets, online registry sources, uploaded
  Dockerfiles and existing SIF paths. Reuse Local base images and project assets,
  support language versions and target architecture, retain portable Dockerfiles
  and image metadata, build inside a temporary local Docker Sandbox, and convert
  uploaded archives to SIF on the allocated compute node.
- Add isolated scheduler, source selection, build/cache, project preservation,
  connection, cancellation and UI regressions. Actual amd64 Python 3.13 and R 4.5
  image builds and archive exports passed on Apple silicon.
- Fix the emulated R ICU loading error by aligning affected x86-64 libraries in
  R's runtime dependency graph for 16 KiB page compatibility. Preserve executable
  code, SONAMEs and dependency names, record before/after hashes, and replace
  files atomically. Include the repair in the portable Dockerfile and invalidate
  older image caches. Gate new R image builds on actual ICU collation, CSV, PDF
  and Cairo PNG checks; the regression passed five consecutive times with the
  launcher's emulator. Interactive VS Code/Codex, Python/R notebook UI sessions
  and Windows end-to-end validation remain outstanding.

- Default remote projects to `~/sandboxes`, partition to `cluster` and CPU count
  to one. Remove the Slurm account control and submit with the user's default
  account. Replace the single data mount with comma-separated additional folders
  mounted at their full paths; tolerate separator whitespace, empty entries and
  duplicates. Add comma-separated module names, loading them for login-node
  Slurm operations and compute jobs while preserving advanced site setup.

- Fix the Python first-run resolver failure found on a live HPC: bootstrap
  pip 25.2, pip-tools 7.5.1 and click 8.2.1 together, and include resolver
  dependencies in the generated hashed lockfile and installation history.

- Carry container SSH through a login-node `srun --jobid` relay, without compute
  host SSH, address lookup or direct-network fallback. Bind the service to loopback,
  combine the SSH environment settings so the container marker is preserved, and
  use server-selected temporary storage for native package builds. Respect
  `SLURM_TMPDIR`/`TMPDIR`/`TMP`/`TEMP`, use Apptainer work directories and propagate
  the temporary location into SSH sessions; clean the launcher-owned subfolder
  on normal exit/termination.

- Validate live Python 3.13.18 and R 4.5.3 workspaces on an x86-64 Slurm HPC
  with Apptainer 1.2.5: upload, compute-node conversion, first-run package setup,
  additional mounts, container SSH, Codex CLI startup, reconnect and cancellation.
  The Slurm relay passed concurrent SSH connections and a 1 MiB binary transfer
  on a one-CPU Slurm 19.05 allocation without compute-node SSH. Native R passed
  ICU, CSV, PDF and PNG checks and renv verification using site-selected temp
  storage, including a path with spaces. Run R
  bootstrap verification with `--vanilla` to avoid repeated startup activation.

- Add configurable server image storage, defaulting to
  `~/.local/share/vscode-sandboxes/images`, with atomic `metadata.json`, a server
  image dropdown and automatic cache lookup before building. Reuse ready SIFs
  without rebuilding, transferring the image or converting it again. Separate
  image identity from project-template revisions, serialize concurrent conversion
  and remove the staged archive after success.
- List launcher-managed running/queued sandboxes, attach to an existing allocation
  (including authorizing another laptop's public key), stop a selected allocation
  and leave a job running while preparing a new sandbox. Check the Slurm job ID,
  owner token and user-owned metadata before authorization or cancellation.
- Live Python/R cache reuse passed on the test HPC with no image build/transfer/
  conversion, including allocation discovery, key authorization, container runtime,
  reconnect, concurrent connections and binary-transfer verification.

### Release validation

The complete Go suite and JavaScript, Python and shell regression suites passed
on macOS. The Python/R live Slurm checks described above passed on
hpc.bioinformatics.studio with Apptainer 1.2.5; these reused cached SIFs.
The macOS DMG was rebuilt and its checksum, version metadata, embedded payloads,
Applications shortcut and visual installation assets verified. The Windows
installer was cross-compiled with version 1.2.0 metadata.

A clean Windows 11 installation/end-to-end run, authenticated provider calls,
and full interactive VS Code/notebook first-run tests have not been completed.
The dedicated graphical R session regression remains outstanding; live R runtime,
ICU, CSV, PDF/PNG and offline computation checks are recorded above.

### SHA-256 checksums

```
ee8143e09eb024351c2de425910d38ddef2f8c0c60db834ba9a3f4da51820393  VS-Code-Sandboxes.dmg
1a2458941e4f0641f538e069e54ff7c9be22fbf3069de0ff22f81546d3aa9b98  VS-Code-Sandboxes.exe
b66258f4eeb4bb82f9da208d547c195d46f8535e96750d8720a43058e3c4cf0a  LICENSE
6d3c58cbf8092ffce8589b92d8a5e71ec3ff9b03f609bdba6f3b81b68fb2222c  THIRD-PARTY-NOTICES.txt
2f704641e01eaa72ef7d14bf8bb55fe895d97426e224d69e53dd4d77f69ec6b6  DISCLAIMER.txt
```

[Full changes](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/compare/v1.1.2...v1.2.0)

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
