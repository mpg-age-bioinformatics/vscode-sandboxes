# VS Code Sandboxes

Downloadable macOS applications and Windows executables that create reproducible development environments
with Docker Sandboxes and open them in Visual Studio Code.

Each sandbox keeps project files on your computer while running the development tools,
language runtime, and selected coding agent in an isolated sandbox.

## Available sandboxes

### Python Sandbox

Python Sandbox creates a project pinned to the Python version you choose. It sets
up a project-local virtual environment, Jupyter support, VS Code extensions, and
either Codex or Claude Code.

### R Sandbox

R Sandbox creates a project pinned to the R major/minor series you choose. It
sets up a project-local R library with `renv`, R language tooling, Quarto, VS Code
extensions, and either Codex or Claude Code.

### Bioinformatics Sandbox

Bioinformatics Sandbox creates a language-neutral scientific project for mixed Python,
R, notebooks, command-line tools, and containerized workloads. It provides the
code/data layout, VS Code tooling, and either Codex or Claude Code.

## Before you install on macOS

Docker Sandboxes currently requires an Apple silicon Mac running macOS Sonoma
14 or newer. Install and configure:

- [Git](https://git-scm.com/download/mac);
- for **Python Sandbox** and **R Sandbox** only, the Docker CLI and a running
  Docker engine, such as
  [Docker Desktop](https://www.docker.com/products/docker-desktop/);
- [Docker Sandboxes](https://docs.docker.com/ai/sandboxes/install/) with the
  `sbx` command, version 0.39.0 or newer;
- [Visual Studio Code](https://code.visualstudio.com/docs/setup/mac) for macOS;
  and
- access to the coding agent you intend to use: Codex or Claude Code.

Sign in once with `sbx login`. If you use Docker Desktop, start it before opening
a sandbox app. The first setup also requires an internet connection to download
the setup files, container images, and VS Code extensions.

The setup initializes a Git repository and makes an initial commit. Configure your
Git name and email first if you have not already done so:

```bash
git config --global user.name "Your Name"
git config --global user.email "you@example.com"
```

## Install Python Sandbox

1. Open the latest Python Sandbox release on this repository's GitHub **Releases**
   page.
2. Download `Python-Sandbox.dmg`.
3. Double-click the downloaded DMG.
4. Drag **Python Sandbox** onto the **Applications** shortcut.
5. Eject the Python Sandbox disk image.

## Install R Sandbox

1. Open the latest R Sandbox release on this repository's GitHub **Releases**
   page.
2. Download `R-Sandbox.dmg`.
3. Double-click the downloaded DMG.
4. Drag **R Sandbox** onto the **Applications** shortcut.
5. Eject the R Sandbox disk image.

## Install Bioinformatics Sandbox

1. Open the latest Bioinformatics Sandbox release on this repository's GitHub
   **Releases** page.
2. Download `Bioinformatics-Sandbox.dmg`.
3. Double-click the downloaded DMG.
4. Drag **Bioinformatics Sandbox** onto the **Applications** shortcut.
5. Eject the Bioinformatics Sandbox disk image.

### Open an unsigned sandbox app

The macOS applications are currently unsigned and not notarized. On first
launch, macOS may show a warning with only **Move to Trash** and **Done**. For an
app downloaded from this repository's GitHub Release:

1. Click **Done**. Do not move the app to Trash.
2. Open **System Settings** → **Privacy & Security**.
3. Scroll down to **Security** and find the message that the sandbox app was
   blocked.
4. Click **Open Anyway**. This button is normally available for about one hour
   after the blocked launch attempt.
5. Authenticate when prompted, then confirm **Open** in the new warning dialog.

macOS records an exception, so this is normally required only once per app. If
**Open Anyway** is absent, try to open the app once more and immediately return
to **Privacy & Security**. Do not override the warning if the app came from an
untrusted source or macOS explicitly reports detected malware; download a fresh
copy from the repository's GitHub Release and verify its checksum instead.

## Install on Windows

Windows executables are available only for 64-bit Intel/AMD Windows 11 systems
with Windows Hypervisor Platform enabled. Windows 10, Windows Server, ARM64
Windows, and systems without usable hardware virtualization are not supported by
Docker Sandboxes.

Install these prerequisites first:

- [Git for Windows](https://git-scm.com/download/win), including Git Bash;
- the Windows OpenSSH Client optional feature;
- [Docker Sandboxes](https://docs.docker.com/ai/sandboxes/install/) 0.39.0 or
  newer, then sign in with `sbx login`;
- [Visual Studio Code](https://code.visualstudio.com/) and access to Codex or
  Claude Code; and
- for **Python Sandbox** and **R Sandbox** only, a running Docker engine on a
  Windows edition and build supported by
  [Docker Desktop](https://docs.docker.com/desktop/setup/install/windows-install/).

Use a project folder on a local Windows drive. Network shares and WSL UNC paths
are rejected because they do not provide the local filesystem behavior required
by mounted sandbox workspaces. Python Sandbox and R Sandbox require Docker
Desktop to be running Linux containers.

Bioinformatics Sandbox does not require Docker Desktop; workload containers use
the private Docker daemon supplied inside Docker Sandboxes.

Download the appropriate file from the GitHub Release:

- `Python-Sandbox.exe`;
- `R-Sandbox.exe`; or
- `Bioinformatics-Sandbox.exe`.

The executables are currently unsigned. Windows SmartScreen may show an
unrecognized-app warning. Run an executable only if it came from this repository's
GitHub Release. The launcher checks compatibility and prerequisites before creating
or modifying a project; it does not install Docker, enable Windows features, or
request administrator privileges.

If Windows blocks the downloaded executable:

1. Right-click the downloaded `.exe`.
2. Select **Properties**.
3. Under **General → Security**, select **Unblock**.
4. Select **Apply**, close Properties, and run the executable again.

## Create a Python project

1. Open **Python Sandbox** on macOS, or double-click `Python-Sandbox.exe` on Windows.
2. Review the prefilled Python version, coding agent, and project folder in the
   setup form, then choose **Setup**. On Windows, **Browse** can select an
   existing project folder; an edited path is created if it does not exist.
3. Use a Python version in `major.minor` or `major.minor.patch` form, such as
   `3.13` or `3.13.2`. The sandbox uses the corresponding major/minor series.
4. The project folder is created if it does not exist.
5. Wait while the application prepares the repository, builds the sandbox,
   configures SSH, installs the VS Code extensions, and opens the project in
   Visual Studio Code.

The initial build can take several minutes. Keep the Terminal window open until
it reports that setup finished or displays an error.

## Create an R project

1. Open **R Sandbox** on macOS, or double-click `R-Sandbox.exe` on Windows.
2. Review the prefilled R version, coding agent, and project folder in the setup
   form, then choose **Setup**. On Windows, **Browse** can select an existing
   project folder; an edited path is created if it does not exist.
3. Use an R version in `major.minor` or `major.minor.patch` form, such as
   `4.5` or `4.5.1`. The latest available Rocker patch for that major/minor
   series is selected.
4. The project folder is created if it does not exist.
5. Wait while the application prepares the repository, builds the R environment,
   configures SSH, installs the VS Code extensions, and opens the project.

The initial R image build may take several minutes because it includes R package
build tools and language-server dependencies.

## Create a bioinformatics project

1. Open **Bioinformatics Sandbox** on macOS, or double-click `Bioinformatics-Sandbox.exe` on Windows.
2. Review the prefilled coding agent and project folder in the setup form, then
   choose **Setup**. On Windows, **Browse** can select an existing project
   folder; an edited path is created if it does not exist.
3. The project folder is created if it does not exist.
4. Wait while the application prepares the repository, creates or reconnects to
   the sandbox, configures SSH, installs the scientific VS Code extensions, and
   opens the project.

Workload containers run on the Docker daemon inside the sandbox, not on the host
Docker socket.

## What is created

The selected project directory contains:

```text
project/
├── code/       # Source code, notebooks, launcher, and container recipe
├── data/       # Project data
├── skills/     # Downloaded sandbox setup skills; ignored by Git
├── .vscode/    # VS Code configuration
├── AGENTS.md
└── .git/
```

Every Windows project receives an agent-specific executable in `code/`:
`Run Python Sandbox.exe`, `Run R Sandbox.exe`, or
`Run Bioinformatics Sandbox.exe`. It is committed with the project.

The project is mounted directly into the sandbox, so edits made in VS Code are
saved on your computer. The Python virtual environment is stored as `.venv/` in the
project and is configured as VS Code's interpreter.

R projects additionally contain `.r-library/`, which is their project-local R
package library. Manage its dependencies with `renv`.

## Open the project again

After the first setup, do not run the installer again on Windows. Open the
project's `code` folder and double-click the generated runner:

- **Run Python Sandbox.exe**;
- **Run R Sandbox.exe**; or
- **Run Bioinformatics Sandbox.exe**.

The runner already contains the agent selected during setup. It reconnects to the
existing sandbox when available and opens the project through VS Code Remote-SSH.

On macOS, double-click the generated **Run Python Sandbox.app** for Python.
For an existing R or bioinformatics project, reopen the installed sandbox app and
enter the same setup values and project directory.

The command-line equivalents, run from the project root, are:

```bash
./code/run-python-sandbox.sh codex
./code/run-r-sandbox.sh codex
./code/run-bioinformatics-sandbox.sh codex
```

Use `claude` instead of `codex` when the project was configured for Claude Code.

## Working in the sandbox

- Put source files and notebooks in `code/`.
- Put datasets in `data/`.
- Install Python packages into the project `.venv`, not into the host OS.
- Install R packages into the project `.r-library/` with `renv`, not into the host OS.
- Build and run specialized workload containers only inside Bioinformatics Sandbox.
- Keep dependency declarations and lock files under version control.
- Use the integrated VS Code terminal after the Remote-SSH window opens; commands
  there run inside the sandbox.
- Authenticate the selected coding agent when it requests access. Do not store
  credentials in the repository.

## Troubleshooting

### The app cannot be opened

If the warning offers only **Move to Trash** and **Done**, click **Done**, then
use **System Settings** → **Privacy & Security** → **Security** → **Open Anyway**.
Confirm that the app came from this repository's GitHub Release before
overriding Gatekeeper.

### `Git is required`

Install Git, reopen Terminal, and verify that `git --version` works.

### `sbx is not installed`

Install or update Docker Sandboxes and verify:

```bash
sbx version
sbx login
```

The sandbox apps require Docker Sandboxes CLI 0.39.0 or newer and an authenticated
Docker Sandboxes session.

### `Docker CLI is required`

Python Sandbox and R Sandbox require a running Docker engine in addition to
Docker Sandboxes. Confirm that Docker Desktop supports your operating-system
edition and version, start it, and run `docker version`. Bioinformatics Sandbox
does not require Docker Desktop.

### Visual Studio Code is not found

Install Visual Studio Code for your operating system and run the sandbox app again.

### Setup stops while making the Git commit

Configure `user.name` and `user.email` as shown above, then run the application
again with the same project directory. Existing matching setup files are
preserved.

### A project file already exists with different contents

The setup refuses to overwrite conflicting files. Review or move the reported
file, then retry. Back up important work before changing an existing project.

## For developers

Build, testing, pull-request, and GitHub Release instructions are in
[CONTRIBUTING.md](CONTRIBUTING.md).
