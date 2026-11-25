# Apptainer agent containment

This describes the **Remote** Slurm/Apptainer implementation in this repository.
Local Docker Sandboxes uses a separate microVM/container architecture. These
Apptainer flags do not change Local's isolation or network controls.

## Reference and scope

The reference is [MPCDF's responsible-agent guidance](https://docs.mpcdf.mpg.de/faq/hpc_software.html#how-can-i-run-ai-coding-agents-responsibly-on-mpcdf-systems)
and its [AI CLI Agents Container, revision `3c61715acd7c18edd598d5b517af58a10322253c`](https://gitlab.mpcdf.mpg.de/mpcdf/ai-cli-agents-container/-/tree/3c61715acd7c18edd598d5b517af58a10322253c),
reviewed on 2026-11-20. We reuse that revision's strict
`apptainer/seccomp-agents-noptrace.json` as
`internal/startui/remote_assets/seccomp-agents.json`, and its Slurm command
explanation script. Their MIT copyright and permission notice are retained in
`THIRD-PARTY-NOTICES.txt` and distributed with our binaries and generated images.
The shell integration, mount validation, startup verification and VS Code/Slurm
transport integration described below are our implementation.

The unmodified strict profile's SHA-256 is
`c08825a0dfc057ad7aad78ef82e702c8282be57b0fc3886c7b6156fc9e934eab`.

The goal is to reduce access to the HPC account's unrelated files, credentials,
processes and privilege-escalation interfaces. This is defense in depth with a
shared host kernel, not a VM boundary, an MPCDF certification, or a guarantee of
equal security on every cluster. The actual runtime/kernel/site configuration
matters. Trusted images and a maintained host remain necessary.

## Host control code and local authentication hardening

Trusted launcher Python runs with `python3 -I` (including scripts supplied over
stdin, inline commands, catalogs, endpoint setup and the host watchdog). Isolated
mode excludes the working directory and user site-packages and ignores Python
import-path environment overrides. This prevents an agent-writable project file
such as `pathlib.py` or `json.py` from executing as a host-side standard-library
import. Cluster module initialization and executable PATH remain explicitly
trusted administrator/user inputs; do not resolve executables from project paths.

The desktop API capability is delivered to the native window as JSON over an
inherited stdin pipe, never argv or environment variables. macOS uses a
nonpersistent WKWebView store and restricts navigation to the exact launcher
origin and capability path. Windows receives the same pipe bootstrap, creates a
user-only ACL directory before writing a temporary bootstrap HTML file, and opens
Edge InPrivate using only that file path in argv. Its private profile/bootstrap
are removed when the window closes (an abrupt OS/process termination can leave
the ACL-protected directory behind). These are session capabilities, not saved
SSH passwords. Same-user malware and privileged OS inspection remain outside
this boundary.

SSH passwords, key passphrases and 2FA responses are single-use in-memory
responses. On Unix the askpass HTTP protocol is carried over a filesystem Unix
socket in a mode-0700 temporary directory, with socket mode 0600 and an additional
random request capability. Other Unix accounts cannot connect merely by reading
a process environment. Windows uses its existing random-capability loopback
bridge. These measures do not promise exclusion from OS swap or crash dumps.

Private model endpoints require HTTPS; HTTP is accepted only for literal
loopback IPs or `localhost`. A Remote loopback endpoint is inside the container,
not the laptop. Redirects remain restricted to the original scheme and origin.

Continue remains pinned and checksum-verified. Distribution revision 3 preserves
upstream `allowedWithPermission` policies, replacing older revision-1 packages
that patched automatic approval. Explicit Continue user choices are not reset.
Revision 3 gates the shared hosted-search function, as detailed below.

### Agent web tools

Transport isolation cannot filter requests made by a model provider's hosted
tools. The shared `common/assets/agent-network.py` therefore applies a second,
application-level policy in the allocation-local home. It runs before the
remote SSH service and again after private-endpoint configuration on attach or
reconnect, before opening VS Code. Local uses the same helper in the shell
sandbox and the exact nested development container selected by workspace label.
Existing Local projects receive the current helper without regenerating files.
Configuration uses the image's system `/usr/bin/python3` (Python 3.11+ with
`tomllib`), independently of a project's selected Python runtime. Custom/older
images need that interpreter; unsupported or malformed configuration stops
startup rather than silently omitting web-tool settings.

| Agent | Selected sites | No internet |
| --- | --- | --- |
| Codex | Built-in web disabled with `web_search="disabled"` and `tools.web_search=false`; hosted Apps disabled with `features.apps=false` and disabled `apps._default` tools | Same hosted-web/Apps disable |
| Claude Code | A `PreToolUse` command hook rewrites/intersects `WebSearch.allowed_domains`; `WebFetch` checks the requested URL against exact/wildcard hosts and ports | Hook exits 2 for both web tools |
| Continue 2.0.0 | Revision-3 guard disables the shared hosted-search function used by `search_web` and `@web`; direct fetch remains transport-controlled | Same guard denies hosted search |
| VS Code Chat | Remote Machine settings supply `github.copilot.chat.anthropic.tools.websearch.allowedDomains` for Anthropic BYOK search | Anthropic BYOK search disabled |

The Claude hook deliberately supplies no `permissionDecision="allow"`, so
normal permissions still apply. Existing unrelated hooks and permissions are
preserved. Explicitly disabled Claude hooks cause restricted startup to fail.
Codex configuration is parsed as TOML before updates; original comments are
kept in an owner-only backup. Managed key snapshots support Open-mode restore
without replacing endpoint/model settings. Continue's original Marketplace
archive digests are unchanged; the single expected shared-search implementation
is patched before installation and its installed-file hashes recorded. Layout
drift, a missing/malformed Continue policy file or an unknown policy mode denies
hosted search before a provider request. Continue's other tool approvals are not
modified. Old revision-2 packages are upgraded on startup/reconnect.

Search-domain filters include subdomains, and wildcard translation also includes
the base domain. They cannot represent exact network-policy semantics. Explicit
port and IP rules are omitted; an empty translated set or more than 100 domains
disables Claude hosted search. Codex hosted web is always disabled in restricted
and offline modes because a live Code Mode trace retrieved CNN and SAPO pages
outside the configured domain list. The earlier mock-server test verified filter
serialization only and did not establish complete hosted-tool enforcement. Claude's initial WebFetch URL check does not
claim to control provider-side redirects. Chat provider tools other than the
configured Anthropic search, MCP/plugins, provider-managed remote tools and
browser services are not covered by these search filters. The general VS Code fetch/browser filter is APPLICATION-scoped in the tested
1.147 build; writing it in remote Machine settings would be ignored. Enforcing
that filter needs a desktop-wide policy and is not claimed here. User/project
settings and tool runtime versions can affect editor-side behavior. All of these files
are writable by the sandbox user: this is application configuration, not a
tamper-resistant service boundary. Direct sandbox egress remains independently
enforced by Docker or the Apptainer network namespace/gateway.

An additional live trace, after built-in search was disabled, showed
`mcp__codex_apps__search_service_web_run` fetching an unlisted site. The helper
therefore disables the entire hosted Apps integration with `features.apps=false`,
`apps._default.enabled=false`, `apps._default.default_tools_enabled=false` and
`apps._default.open_world_enabled=false`. Other connected-app tools are also
unavailable in restricted/offline modes. An installed-extension app-server probe
confirmed the effective flag and that its MCP runtime inventory contained no
`codex_apps` server. Reload the extension and replace existing conversations to
discard their old tool catalogs.

References: [Codex configuration](https://learn.chatgpt.com/docs/config-file/config-reference),
[search domain semantics](https://developers.openai.com/api/docs/guides/tools-web-search#domain-filtering),
[Claude hooks](https://code.claude.com/docs/en/hooks), and
[VS Code agent settings](https://code.visualstudio.com/docs/agents/reference/ai-settings).

## Runtime command and enforcement

`remoteAsset` embeds the common `remote_assets/security.sh` helpers in the
submission, batch and SSH relay scripts. The batch script writes the bundled
strict profile into the allocation's launcher control directory. The workspace
is started using this argument set, with each bind passed as a separate argv
element rather than shell-evaluated user input:

```text
apptainer exec
  --cleanenv --containall --no-init --no-eval
  --no-privs --drop-caps ALL
  --no-mount hostfs,cwd,bind-paths,sys
  --security seccomp:<allocation-control>/seccomp-agents.json
  --workdir <server-temporary-directory>/runtime
  --home <server-temporary-directory>/home:<HPC-passwd-home>
  --bind <resolved-project>:<requested-project>:rw
  --bind <allocation-control>:/run/vscode-launcher:ro
  --bind <allocation-control>/runtime:/run/vscode-launcher/runtime:rw
  --bind <server-temporary-directory>/files:<container-temp>:rw
  --env TMPDIR=<container-temp>,TMP=<container-temp>,TEMP=<container-temp>
  --pwd <requested-project>
  [--bind <resolved-additional-folder>:<requested-folder>:ro ...]
  [--nv and explicit Slurm GPU-selection variables]
  <image.sif> /bin/bash /run/vscode-launcher/container.sh ...
```

`--containall` requests filesystem, PID, IPC and environment containment. The
container has a minimal device tree and a private process view; `/proc` is not
the host process list. `--no-init` avoids Apptainer's shim init; Apptainer still
creates the PID namespace. The batch shell retains termination forwarding and
cleanup of its own temporary directory. `--no-eval` disables Apptainer's shell
evaluation of environment/command arguments. `--no-mount` disables automatic
host filesystem, working-directory and administrator-configured bind-path
mounts, and the host `/sys` mount. Explicit mounts remain active. These flags
are mandatory; we do not retry with weaker flags if the site rejects them.

Apptainer is invoked through `env -i`. The batch wrapper passes only `PATH`,
`HOME`, `USER`, `LOGNAME`, `TMPDIR`, `APPTAINER_TMPDIR` and
`APPTAINER_CACHEDIR`. Its conversion step uses the same wrapper. In particular,
host API keys, `SSH_AUTH_SOCK`, loader injection variables, and inherited
`APPTAINER_BIND*`, `APPTAINERENV_*`, `SINGULARITY_*`, overlay/fakeroot/security
switches are not forwarded. Host `HOME` is available to Apptainer for its own
cache lookup; inside the workspace `--home` supplies the disposable home.
Image-defined environment/startup files are still part of the trusted image.
The transport helper uses its own `env -i` invocation with only `PATH`, `HOME`
and `USER`, followed by the same containment/capability/seccomp flags.

Before project initialization or SSH startup, `verify-security.py` checks
`/proc/self/status`: `CapInh`, `CapPrm`, `CapEff`, `CapBnd` and `CapAmb` must all
be zero, `NoNewPrivs` must equal 1, and `Seccomp` must equal 2 (filter mode).
It then verifies that `unshare(0)` and an invalid `ptrace` request return
`EPERM`, and that Python threading still works. A failure aborts startup.
`NoNewPrivs` is checked as an effective kernel property, rather than inferred
from a successful command-line parse. Children, including the SSH service,
VS Code Server, extensions, agents and terminals, inherit these restrictions.

## Filesystem boundary

The project is mounted read/write at its requested absolute path. Additional
folders are explicit read-only mounts. Source paths are resolved on the compute
node before binding, while their requested destinations are preserved. The
project remains the writable persistent workspace. Its files can be read,
changed, deleted and transmitted by the agent. Additional mounts protect
against writes, not disclosure of their contents.

The fake home is allocation-local, backed by the server's temporary space.
VS Code Server, remote extensions, agent sign-ins and private-endpoint settings
are placed there. We do not mount the real HPC home, its SSH keys, shell startup
files, agent sockets, Docker socket, Slurm configuration, or Munge socket.
Apptainer may still provide normal identity and DNS metadata such as passwd,
group and resolv.conf entries; hiding the host home is not identity anonymity.

Submission and compute startup both validate the logical and resolved paths:

- Reject the real home or any ancestor, including `/`.
- Reject paths inside any top-level hidden home entry, such as `.ssh`,
  `.config`, `.local` or `.bashrc`; reject writable `~/bin` too.
- Reject container system/runtime destinations under `/proc`, `/sys`, `/dev`,
  `/run`, `/tmp`, `/var/tmp`, `/etc`, `/usr`, `/bin`, `/sbin`, `/lib`, `/lib64`.
- Reject overlap with launcher state/control paths, the image file, or the
  global watchdog state. Existing checks also reject overlap between additional
  read-only folders and the writable project, including symlink aliases.

This is deliberately stricter than allowing arbitrary paths through the UI.
Read-only additional folders can still contain sensitive data: choosing them
is an explicit exposure decision. We do not scan their contents for secrets.

The complete allocation control directory is visible **read-only** at
`/run/vscode-launcher`. This includes scripts, module setup text, allocation
identity, authorized public keys and staged project files. Do not put credentials
in module initialization text. Module initialization and host `PATH` are trusted
host setup: do not source scripts or resolve cluster tools from an agent-writable
project directory. A nested writable `runtime` mount contains only
this allocation's SSH keys/configuration, readiness/phase status and connection
observations. Compatibility symlinks in the host control directory point to
these runtime files. An agent can alter its own runtime data but cannot write
the batch script, relay, watchdog code or module commands that are later run on
the host. Runtime observations are not a tamper-proof audit log against the
same-UID agent; the watchdog does not use them as host command text.

Temporary space selection still honors `SLURM_TMPDIR`, `TMPDIR`, `TMP`, `TEMP`,
then the server's `mktemp` default. A unique allocation subdirectory holds home,
temporary files and Apptainer work directories. When the temporary path has
spaces, R receives a space-free container alias under the runtime mount. Cleanup
does not follow symlinks into persistent storage.
Cleanup is best effort: forced termination or a host/filesystem failure can
leave temporary directories behind. This implementation does not securely erase
credentials or override the site's temporary-storage retention policy.

## Strict syscall policy

The vendored profile is an OCI seccomp **denylist**, with default action
`SCMP_ACT_ALLOW`, native x86-64 and AArch64 architectures, and explicit errno
rules. It blocks these categories:

| Category | Syscalls |
| --- | --- |
| Namespace changes | `unshare`, `setns` |
| Mount/root manipulation | `mount`, `umount2`, `pivot_root`, `move_mount`, `open_tree`, `fsopen`, `fsconfig`, `fsmount`, `fspick`, `mount_setattr`, `chroot` |
| Cross-process access | `ptrace`, `process_vm_readv`, `process_vm_writev`, `pidfd_getfd`, `kcmp` |
| Kernel attack surface | `bpf`, `perf_event_open`, `userfaultfd`, `io_uring_setup`, `io_uring_enter`, `io_uring_register` |
| Keyrings | `keyctl`, `add_key`, `request_key` |
| Modules/kexec | `init_module`, `finit_module`, `delete_module`, `query_module`, `kexec_load`, `kexec_file_load` |
| Administrative/obsolete interfaces | `reboot`, `swapon`, `swapoff`, `settimeofday`, `stime`, `clock_settime`, `clock_settime64`, `acct`, `quotactl`, `vhangup`, `sethostname`, `setdomainname`, `ioperm`, `iopl`, `nfsservctl`, `vmsplice`, `fanotify_init`, `lookup_dcookie`, `uselib`, `ustat` |
| File-handle resolution | `open_by_handle_at`, `name_to_handle_at` |
| Audit sockets | `socket` only when domain is `AF_NETLINK` (16) **and** protocol is `NETLINK_AUDIT` (9) |

The non-socket rules request `EPERM`. The audit-socket rule requests `EINVAL`;
older Apptainer/libseccomp combinations may not honor per-rule errno values.
We check denial, not a guarantee of the requested errno for that socket rule.
Ordinary TCP/DNS, pipes, file IO, fork and threading are allowed. Native
`gdb`/`strace`/ptrace-based debuggers and io_uring-dependent workloads are not
supported by this strict profile. There is no UI switch or silent fallback to
the weaker upstream profiles.

As upstream explains, **`clone` and `clone3` are intentionally not filtered**:
blanket denial can break glibc thread creation, Node, Python and the agent
runtimes. Consequently this policy does not eliminate every possible nested
user-namespace creation path. It is not a syscall allowlist. Native x86-64 and
AArch64 are the supported ABIs; compatibility ABIs are not separately validated.

## Slurm, SSH, watchdog and networking

Slurm submission, inventory, cancellation and byte transport run on the host
side, outside the agent environment. The rootless SSH service listens only on
compute-node loopback, uses a per-allocation authorized key, pinned Ed25519
host key, strict host verification, no passwords and no SSH agent forwarding.
No direct SSH connection to a compute node is introduced.

Large launch scripts and their quoted arguments travel over SSH stdin, rather
than the OS command line, avoiding Windows command-length limits.

The SSH transport remains login SSH → identity-checked `srun` → a short
Apptainer TCP relay. This helper has the strict policy but no project, real
home or control-directory bind. This is the Open-mode transport. Selected sites and No internet instead use
login SSH → identity-checked `srun` → a fixed host-side Python byte relay → a
pathname UNIX socket → a container-side bridge → the container's loopback SSH
service. UNIX pathname sockets work across network namespaces. No compute-node
SSH, `nsenter`, host TCP listener, root privilege or general host command
execution is exposed to the workspace. The relay applies raw PTY mode for older
Slurm versions, preserving SSH binary traffic and half-close behavior.

The singleton watchdog inspects the correct service across PID namespaces.
For new allocations, the container's initial PID/start/namespace record is only
an untrusted discovery hint. Before normal waiting, the host batch process runs
`watchdog.py anchor` to verify the server's UID, executable, start tick, namespace
and ancestry under that exact Apptainer launcher. It writes `server-identity.json`
in the control directory outside the writable runtime bind. `watchdog-policy`
requires that anchor before subsequent host probes accept liveness.

Probes use this host-written identity rather than the container's mutable
`ssh-server.json`; authenticated process titles also require real sshd/session
executables and an ancestry chain containing only those executables back to the
anchored server. Shells with forged process titles are not connections. Ready
clock values come from the host anchor; future/non-finite timestamps cannot
extend idle time. For anchored allocations, writable SSH activity files are
ignored. Generated SSH config uses `PermitLocalCommand yes` and a client-side
`LocalCommand` that runs only after successful container authentication. It
reuses the laptop's outer login to write `host-activity.json` in the read-only
control directory. That callback uses `ssh -n` and redirects its stdout to stderr:
it must never consume a workspace script/credential payload from the parent's
stdin or contaminate the container proof/model JSON output. This preserves short sessions between one-minute polls
without granting the agent permission to forge its own host heartbeat. The host
command requires the allocation identity and an authenticated host SSH session. A probe that cannot establish process identity retains the allocation
and retries, rather than cancelling a potentially active editor.

Legacy allocations have no host anchor and retain compatibility observation;
stop and recreate them to obtain the new boundary. No second watchdog daemon,
`nsenter` or direct compute SSH is introduced. Host-account compromise and
cooperating processes elsewhere in the same HPC account remain outside the
container threat model.

New generated images place explanation stubs for Slurm commands before normal
system commands in `PATH`, following MPCDF's workflow: write job scripts in the
workspace, submit them from a regular host shell. These stubs are a usability
guard, not an authorization boundary: binaries at other paths or credentials
deliberately placed in the project are not neutralized by a `PATH` wrapper.

Remote internet mode is saved per allocation. Old profiles default to **Open**;
existing running allocations retain their original policy. Reconnecting does
not upgrade an Open allocation. Changing mode/allowlist requires a new job.

### No internet and Selected sites

Both modes add these arguments to the workspace command:

```text
--net --network none
--bind <allocation-temporary>/ssh:/run/vscode-ssh:rw
# Selected sites only:
--bind <allocation-temporary>/network:/run/vscode-network:ro
```

Apptainer must support an unprivileged private network namespace with its `none`
network. No sudo, firewall changes, `slirp4netns`, privileged CNI bridge or
administrator-installed daemon is requested. A site can disable the necessary
user namespaces or network feature; that causes startup failure, never fallback
to Open. Cluster policy still takes precedence. Inside the workspace,
`network.py verify` checks that the network namespace differs from the batch
host, only `lo` exists, and direct IPv4/IPv6 connection attempts fail. The existing
capability, seccomp and NoNewPrivs verification also runs before project setup.
The process cannot regain the host network via namespace changes under the
strict seccomp policy. No direct external DNS, TCP or UDP route is provided.

For both isolated modes, the batch script creates an allocation-local hosts
file and explicitly binds it read-only at `/etc/hosts`. It contains only
`127.0.0.1 localhost` and `::1 ip6-localhost ip6-loopback`; host/cluster DNS
mappings are not imported. `localhost` deliberately resolves to IPv4 loopback,
matching the Codex OAuth listener. This avoids Apptainer's empty generated hosts
file with `--network none`, which made VS Code's remote callback tunnel fail
with `getaddrinfo EAI_AGAIN localhost` despite working numeric loopback sockets.
External DNS and routing remain blocked; this adds no gateway exception.

**No internet** creates no egress gateway. Local loopback services (SSH,
notebooks and debugger listeners) still work. The VS Code connection transports
editor traffic through SSH; this setting does not prevent a human from
copying/downloading workspace data through their editor or laptop. Online
project dependency resolution and skills cloning are explicitly skipped;
Python creates/reuses `.venv` and runs `pip check`, R creates/reuses `.r-library`.
Prepare dependencies in the image/project before selecting offline mode. No
private model endpoint is accepted in this mode.

**Selected sites** starts one `network.py gateway` process for this allocation
in the batch host's network/PID namespace. It loads the allowlist once from the
launcher-owned profile before workspace startup. It runs as the HPC user,
within the allocation, and is terminated by the batch cleanup. It executes no
project code. It listens on a mode-0600 pathname UNIX socket in the unique,
mode-0700 allocation temporary tree. That directory is mounted read-only in the
workspace; the workspace cannot replace its socket, alter its policy, signal
its process through the private PID namespace or see the host filesystem.
Socket bind/connect use relative basenames after changing directory, avoiding
AF_UNIX pathname-length failures on long cluster home/scratch paths.

A container-local bridge accepts HTTP proxy connections on a dynamically chosen
loopback port and forwards bytes to that socket. `HTTP_PROXY`, `HTTPS_PROXY`,
`ALL_PROXY`, lowercase equivalents, loopback-only `NO_PROXY` and
`NODE_USE_ENV_PROXY=1` are exported during setup and explicitly included in
sshd's single `SetEnv` directive (OpenSSH ignores later directives), so SSH terminals and VS Code inherit them. Remote Machine
settings set `http.proxy`, `http.proxySupport=override` and loopback exclusions.
Managed private Continue models also receive `requestOptions.proxy`. Node's
built-in environment proxy support needs Node 22.21+ on the 22.x line or Node
24+; the shipped runtime is compatible. A client ignoring proxy settings fails
to connect; unsetting variables does not grant direct network access.

The gateway validates the requested authority before opening an upstream
connection. Exact hostnames/IPv4 addresses and `*.example.org` subdomains are
supported; `**.example.org` also matches the base domain. A rule without a port
allows 80/443; an explicit `:port` permits that port only. Wildcards match at
dot boundaries. Matching uses lowercase names with a trailing dot removed.
DNS resolution happens outside the workspace only after the authority matches.
Every resolved address must be globally routable and non-multicast by default;
private, loopback, link-local and other non-public addresses are rejected. The
connection uses the already-validated numeric address without a second DNS lookup.
The Remote checkbox **Allow internal-network destinations in Selected sites**
persists `allowPrivateNetwork: true` for a new allocation and explicitly permits
non-public addresses; the hostname/port allowlist still applies. Enable it only
for intended internal services. Existing gateways keep their recorded policy.
The gateway supports HTTP absolute-form requests and CONNECT tunnels for HTTPS.
It rejects ambiguous authorities, duplicate/invalid headers, mismatched Host,
transfer-encoding, upgrades, invalid/oversized body lengths and unsafe framing.
HTTP forwards exactly one validated request with `Connection: close`; pipelined
bytes are discarded. CONNECT is a bounded-memory bidirectional byte tunnel.
The gateway limits concurrent connections to 128 and uses connection/header
read deadlines. HTTP request bodies are limited to 64 MiB. It logs no URLs,
headers, bodies, keys or private model traffic.

Private endpoint host/port is automatically appended to a **new allocation's**
allowlist and disclosed in the UI/activity. Credentials are excluded from that
profile. Existing allocations cannot gain new destinations by applying endpoint
credentials: their immutable gateway continues enforcing its original rules.
Default package/vendor lists are visible, editable starting points; the backend
does not silently add package domains. Redirects require their destination to
be separately allowed. SSH Git transport, arbitrary TCP/UDP and direct package
mirrors using other protocols will fail; use HTTPS with allowed hosts.

This is authority/port egress control, not TLS inspection, content filtering or
a guarantee against data exfiltration. CONNECT does not verify TLS SNI/HTTP
Host inside encrypted traffic; servers/shared IPs accepting other names and
allowed forwarding/proxy services can weaken domain restrictions. Wildcards,
shared hosting, user-controlled GitHub repositories, package endpoints and
explicitly allowed private/internal hosts must be treated as destinations that
can receive data. DNS uses the site's resolver and trust model. Shared filesystem access is also outside the network boundary: another host
process can read/forward project files. A pathname UNIX socket deliberately
exposed through a project/additional-folder mount can reach its host service
across network namespaces, just as the launcher's dedicated bridge does.
Read-only bind mounts do not prevent connecting to such sockets. Mount data
folders, not host daemon/proxy sockets, and do not treat this as protection
against cooperating processes elsewhere in the same HPC account. The gateway
itself has the user's host network permissions; it is trusted launcher code,
not a separate privileged firewall or VM. Administrators can enforce stronger
network policy outside the user account.

### Editor provisioning before restricted startup

A separate, strict Apptainer helper uses the host network to download the exact
VS Code Server commit reported by the installed laptop editor and to install
the default remote extensions, including the verified pinned Continue bundle.
It runs before project setup and receives only a fresh disposable home,
disposable temporary storage and read-only provisioning scripts plus a minimal
profile containing the editor commit/extension IDs. It receives **no project,
additional folders, user home, endpoint credentials, authorized keys, module
scripts or allocation control-directory bind**. It does not execute project
setup. Its files are transferred between mounts using cross-filesystem-safe
moves. Server downloads use HTTPS and archive traversal/link checks; extension
VSIX files are resolved/downloaded through Microsoft's public Marketplace API
and publisher CDN, with identity/version checks against their manifest.
Dependencies are installed before each extension; packs are expanded after
their parent to handle Python/debugpy's pack/dependency cycle. Stable versions
for the actual Linux architecture are selected. Local VSIX files are installed
through the official server CLI (which checks compatibility), with its recursive
pack/dependency fetching disabled because those artifacts are prepared here.
After copying the verified Continue distribution, the helper registers its
exact 2.0.0 directory in the fresh home's default `extensions/extensions.json`
inventory with platform, pinned and machine-scoped metadata; other inventory
entries are preserved. It verifies that the actual `code-server --list-extensions`
recognizes Continue. Copying files alone after other CLI installs would leave
Continue absent from this inventory. Downloading VSIX files through the public
API also handles clusters where the server CLI's gallery query fails. Downloads retry transient errors with bounded backoff. Extension
installation uses Microsoft's server CLI/Marketplace. This trusts those vendor
artifacts and the image's tools; it is not artifact signing verification.

The prepared home contains both legacy `bin/<commit>` and modern
`cli/servers/Stable-<commit>/server` server layouts. Host VS Code settings set
`remote.SSH.localServerDownload=always` for restricted modes so any additional
Remote SSH bootstrap binary can arrive over SSH from the laptop. This is a
persistent VS Code user setting, also affecting other Remote SSH hosts. Host
image downloads/builds, editor provisioning, VS Code's laptop UI and this SSH
transport are outside workspace egress control. Once the project is mounted,
its first process and all initialization obey the requested mode. There is no
temporary Open workspace bootstrap and no network relaxation on errors.

GPU mode additionally exposes the selected
NVIDIA devices and driver libraries using `--nv`; it widens the device surface.
Slurm GPU visibility variables are forwarded explicitly when supplied. These
variables are not a substitute for administrator-enforced GPU device cgroups.

## What is in a SIF, and what is not

New generated Dockerfiles and the example Apptainer definition include the
strict profile, verifier, Slurm explanation stubs, notices and this document
under `/usr/local/share/vscode-sandboxes/security`. The generated Dockerfile
records the policy version and SHA-256 of the profile. Updating this recipe
changes its cache identity. The exported recipe therefore carries the policy
assets to another build environment.

**The actual kernel protections are launch-time settings.** Merely possessing
the SIF, or running it with plain `apptainer exec image.sif ...`, does not apply
this policy. The launcher supplies its own bundled policy and flags even for
an existing SIF or a selected cached image. SIF content is normally mounted
read-only; we request no writable rootfs, writable overlay, fakeroot, additional
capabilities or setuid permission. We do not sign images, verify a publisher's
signature, scan for vulnerabilities, or make arbitrary uploaded Dockerfiles
trustworthy. Image builds and image-defined setup are executable code.
We do not implement MPCDF's optional npm/pip minimum-release-age build policy,
automatically expose its software/module tree, or add its MPI configuration.
Those are separate supply-chain and site-integration choices, not implicit
properties of this runtime policy.

Already running allocations retain the scripts and flags with which they were
started; reconnecting does not retrofit their namespaces, seccomp filter or
mount table. Stop and create a new allocation to use this implementation.
Existing saved profiles and caches do not exempt a newly started workspace.

## Verification

Automated tests cover argument construction, environment sanitization, unsafe
mount rejection, the strict profile contents, read-only control files, effective
SSH settings and watchdog namespace identity. Live checks use disposable Slurm
allocations on `hpc.bioinformatics.studio` with Apptainer 1.2.5 and cached Python
and R SIFs. The result of those checks is recorded in `CHANGES.md`; unsupported
platforms and untested GPU behavior must not be inferred from CPU results.
The live tests check hidden host-home markers, hostile inherited bind/env
settings, read-only controls/additional folders, writable project/fake home,
distinct PID/IPC namespaces, syscall denials, capabilities, Python fork/threads,
Node worker threads, agent CLI versions and an R runtime smoke check. They do
not authenticate to a model provider or validate interactive VS Code sessions.
An agent's optional nested sandbox may itself require blocked `unshare`/mount
operations; those fail under this policy and are not automatically relaxed.

To repeat the local and opt-in HPC checks:

```sh
go test ./internal/startui -run 'TestRemoteSecurity|TestRemoteWatchdog$|TestRemoteSSHEffectiveEnvironment' -count=1
VSCODE_REMOTE_WATCHDOG_HPC=your-ssh-alias \
VSCODE_REMOTE_WATCHDOG_SIF=/absolute/path/to/python.sif \
VSCODE_REMOTE_WATCHDOG_SECOND_SIF=/absolute/path/to/r.sif \
VSCODE_REMOTE_SECURITY_TEST=1 \
go test ./internal/startui -run '^TestRemoteWatchdog(LinuxHPC|LiveHPC)$' -count=1 -v -timeout=10m
```

The live test submits disposable allocations and cancels only those test jobs.
Its lifecycle check expects no unrelated registered sandboxes to remain. It
advances only its own idle timestamps to exercise cancellation without waiting
24 hours; the production timeout stays unchanged. Build the example definition
from the repository root with
`apptainer build workspace.sif internal/startui/remote_assets/remote.def`.

The network gateway has deterministic malformed-request, policy and UNIX-socket
tests in `scripts/test-remote-network.py`. Opt-in live network validation uses
fresh disposable jobs (without requiring unrelated allocations to stop):

```sh
VSCODE_REMOTE_WATCHDOG_HPC=your-ssh-alias \
VSCODE_REMOTE_WATCHDOG_SIF=/absolute/path/to/workspace.sif \
go test ./internal/startui -run '^TestRemoteNetworkLiveHPC$' -v -count=1 -timeout=25m
```

This downloads editor/extensions in isolated helpers, checks permitted and denied
HTTPS destinations, direct IPv4/IPv6 and proxy bypass, Node fetch, the offline
server CLI and SSH through Slurm. It does not make authenticated provider calls
or substitute for an interactive VS Code first-run test.

Relevant primary runtime documentation:
[Apptainer unprivileged networking](https://apptainer.org/docs/user/1.2/networking.html),
[Node environment proxy support](https://nodejs.org/download/release/v22.23.0/docs/api/cli.html#node_use_env_proxy1),
[Apptainer 1.2 exec flags](https://apptainer.org/docs/user/1.2/cli/apptainer_exec.html),
[bind paths and disabling automatic mounts](https://apptainer.org/docs/user/1.2/bind_paths_and_mounts.html),
and [security options](https://apptainer.org/docs/user/1.2/security_options.html).
