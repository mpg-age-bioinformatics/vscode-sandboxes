# VS Code Sandboxes

Downloadable macOS applications and Windows executables that create reproducible development environments
with Docker Sandboxes and open them in Visual Studio Code.

Each sandbox keeps project files on your computer while running the development tools,
language runtime, and selected coding agent in an isolated sandbox.

## Contents

- [Available sandboxes](#available-sandboxes)
  - [Python Sandbox](#python-sandbox)
  - [R Sandbox](#r-sandbox)
  - [Bioinformatics Sandbox](#bioinformatics-sandbox)
- [macOS](#macos)
  - [macOS requirements](#macos-requirements)
  - [Verify macOS requirements before downloading](#verify-macos-requirements-before-downloading)
  - [Install the macOS applications](#install-the-macos-applications)
  - [Open an unsigned sandbox app](#open-an-unsigned-sandbox-app)
  - [Create projects on macOS](#create-projects-on-macos)
- [Windows](#windows)
  - [Windows requirements](#windows-requirements)
  - [Verify Windows requirements before downloading](#verify-windows-requirements-before-downloading)
  - [Download the Windows executables](#download-the-windows-executables)
  - [Unblock a downloaded executable](#unblock-a-downloaded-executable)
  - [Create projects on Windows](#create-projects-on-windows)
- [Using the generated project](#using-the-generated-project)
  - [What is created](#what-is-created)
  - [Open the project again](#open-the-project-again)
  - [Working in the sandbox](#working-in-the-sandbox)
- [Troubleshooting](#troubleshooting)
- [For developers](#for-developers)

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

## macOS

### macOS requirements

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

Sign in once with `sbx login`.

**Before opening Python Sandbox or R Sandbox, start Docker Desktop and wait until
the Docker engine reports that it is running.** This is required on both macOS
and Windows. Bioinformatics Sandbox does not require the host Docker engine.

The first setup also requires an internet connection to download the setup
files, container images, and VS Code extensions.

The setup initializes a Git repository and makes an initial commit. Configure your
Git name and email first if you have not already done so:

```bash
git config --global user.name "Your Name"
git config --global user.email "you@example.com"
```

### Verify macOS requirements before downloading

Run these checks in Terminal before downloading a sandbox application:

```bash
uname -m
sw_vers -productVersion
git --version
ssh -V
code --version
sbx version
sbx login
sbx setup ssh
sbx diagnose
git config --global --get user.name
git config --global --get user.email
```

`uname -m` must report `arm64`, macOS must be Sonoma 14 or newer, `sbx` must be
0.39.0 or newer, and `sbx diagnose` must finish without failed checks. The
`sbx login` command is interactive; complete the sign-in before continuing.

For Python Sandbox or R Sandbox, also start Docker Desktop and verify the host
Docker engine:

```bash
docker version
docker info --format '{{.OSType}}'
```

The final command must print `linux`. Bioinformatics Sandbox does not require
these two Docker checks. If either Git identity command prints nothing, configure
the missing value before running a launcher.

### Install the macOS applications

#### Python Sandbox

1. Open the latest Python Sandbox release on the
   [GitHub Releases](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/releases)
   page.
2. Under **Assets**, download `Python-Sandbox.dmg`.
3. Double-click the downloaded DMG.
4. Drag **Python Sandbox** onto the **Applications** shortcut.
5. Eject the Python Sandbox disk image.

#### R Sandbox

1. Open the latest R Sandbox release on the
   [GitHub Releases](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/releases)
   page.
2. Under **Assets**, download `R-Sandbox.dmg`.
3. Double-click the downloaded DMG.
4. Drag **R Sandbox** onto the **Applications** shortcut.
5. Eject the R Sandbox disk image.

#### Bioinformatics Sandbox

1. Open the latest Bioinformatics Sandbox release on the
   [GitHub Releases](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/releases)
   page.
2. Under **Assets**, download `Bioinformatics-Sandbox.dmg`.
3. Double-click the downloaded DMG.
4. Drag **Bioinformatics Sandbox** onto the **Applications** shortcut.
5. Eject the Bioinformatics Sandbox disk image.

### Open an unsigned sandbox app

The macOS applications are currently unsigned and not notarized. On first
launch, macOS may show a warning with only **Move to Trash** and **Done**. For an
app downloaded from the
[GitHub Releases](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/releases)
page:

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
copy from the
[GitHub Releases](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/releases)
page under **Assets** and verify its checksum instead.

### Create projects on macOS

#### Create a Python project on macOS

1. Open **Python Sandbox** from Applications.
2. Review the prefilled Python version, coding agent, and project folder in the
   setup form, then choose **Setup**.
3. Use a Python version in `major.minor` or `major.minor.patch` form, such as
   `3.13` or `3.13.2`. The sandbox uses the corresponding major/minor series.
4. The project folder is created if it does not exist.
5. Keep the Terminal window open while the application prepares the repository,
   builds the sandbox, configures SSH, installs the VS Code extensions, and
   opens the project in Visual Studio Code.

The initial build can take several minutes.

#### Create an R project on macOS

1. Open **R Sandbox** from Applications.
2. Review the prefilled R version, coding agent, and project folder in the setup
   form, then choose **Setup**.
3. Use an R version in `major.minor` or `major.minor.patch` form, such as `4.5`
   or `4.5.1`. The latest available Rocker patch for that major/minor series is
   selected.
4. The project folder is created if it does not exist.
5. Keep the Terminal window open while the application prepares the repository,
   builds the R environment, configures SSH, installs the VS Code extensions,
   and opens the project.

The initial R image build can take several minutes because it includes R package
build tools and language-server dependencies.

#### Create a bioinformatics project on macOS

1. Open **Bioinformatics Sandbox** from Applications.
2. Review the prefilled coding agent and project folder in the setup form, then
   choose **Setup**.
3. The project folder is created if it does not exist.
4. Keep the Terminal window open while the application prepares the repository,
   creates or reconnects to the sandbox, configures SSH, installs the scientific
   VS Code extensions, and opens the project.

Workload containers run on the Docker daemon inside the sandbox, not on the host
Docker socket.

## Windows

### Windows requirements

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

**Before opening Python Sandbox or R Sandbox, start Docker Desktop and wait until
the Docker engine reports that it is running.** If Docker Desktop is stopped—or
is using Windows containers instead of Linux containers—the launcher cannot
build the sandbox environment.

### Verify Windows requirements before downloading

Open PowerShell and run these checks before downloading a sandbox executable:

```powershell
[Environment]::Is64BitOperatingSystem
Get-CimInstance Win32_OperatingSystem |
  Select-Object Caption, Version, BuildNumber, OSArchitecture
Get-ComputerInfo -Property HyperVisorPresent
$env:Path = "$env:WINDIR\System32\OpenSSH;$env:Path"
git.exe --version
& "C:\Program Files\Git\bin\bash.exe" --version
& "$env:WINDIR\System32\OpenSSH\ssh.exe" -V
code.cmd --version
sbx.exe version
sbx.exe login
sbx.exe setup ssh
sbx.exe diagnose
git.exe config --global --get user.name
git.exe config --global --get user.email
```

The first command must print `True`, the operating-system details must show
64-bit Windows 11 build 22000 or newer, `HyperVisorPresent` must be `True`, `sbx`
must be 0.39.0 or newer, and `sbx diagnose` must finish without failed checks.
Complete the interactive `sbx login` before running SSH setup and diagnostics.
The temporary `PATH` update makes the checks use Windows OpenSSH rather than
Git's bundled SSH client. If Git Bash was installed somewhere else, replace its
path in the command above. If either Git identity command prints nothing,
configure the missing value before running a launcher.

For Python Sandbox or R Sandbox, start Docker Desktop in Linux-container mode
and also run:

```powershell
docker.exe version
docker.exe info --format '{{.OSType}}'
```

The final command must print `linux`. Bioinformatics Sandbox does not require
these two Docker checks.

### Download the Windows executables

Open the appropriate release on the
[GitHub Releases](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/releases)
page. Under **Assets**, download the appropriate file:

- `Python-Sandbox.exe`;
- `R-Sandbox.exe`; or
- `Bioinformatics-Sandbox.exe`.

### Unblock a downloaded executable

The executables are currently unsigned. Windows SmartScreen may show an
unrecognized-app warning. Run an executable only if it came from the
[GitHub Releases](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/releases)
page. The launcher checks compatibility and prerequisites before creating
or modifying a project; it does not install Docker, enable Windows features, or
request administrator privileges.

If Windows blocks the downloaded executable:

1. Right-click the downloaded `.exe`.
2. Select **Properties**.
3. Under **General → Security**, select **Unblock**.
4. Select **Apply**, close Properties, and run the executable again.

### Create projects on Windows

#### Create a Python project on Windows

1. Double-click `Python-Sandbox.exe`.
2. Review the prefilled Python version, coding agent, and project folder in the
   setup form, then choose **Setup**. **Browse** can select an existing project
   folder; an edited path is created if it does not exist.
3. Use a Python version in `major.minor` or `major.minor.patch` form, such as
   `3.13` or `3.13.2`. The sandbox uses the corresponding major/minor series.
4. The project folder is created if it does not exist.
5. Keep the launcher window open while it prepares the repository, builds the sandbox,
   configures SSH, installs the VS Code extensions, and opens the project in
   Visual Studio Code.

The initial build can take several minutes. Keep the launcher window open until
it reports that setup finished or displays an error.

#### Create an R project on Windows

1. Double-click `R-Sandbox.exe`.
2. Review the prefilled R version, coding agent, and project folder in the setup
   form, then choose **Setup**. **Browse** can select an existing project folder;
   an edited path is created if it does not exist.
3. Use an R version in `major.minor` or `major.minor.patch` form, such as
   `4.5` or `4.5.1`. The latest available Rocker patch for that major/minor
   series is selected.
4. The project folder is created if it does not exist.
5. Keep the launcher window open while it prepares the repository, builds the R
   environment, configures SSH, installs the VS Code extensions, and opens the
   project.

The initial R image build may take several minutes because it includes R package
build tools and language-server dependencies.

#### Create a bioinformatics project on Windows

1. Double-click `Bioinformatics-Sandbox.exe`.
2. Review the prefilled coding agent and project folder in the setup form, then
   choose **Setup**. **Browse** can select an existing project folder; an edited
   path is created if it does not exist.
3. The project folder is created if it does not exist.
4. Wait while the application prepares the repository, creates or reconnects to
   the sandbox, configures SSH, installs the scientific VS Code extensions, and
   opens the project.

Workload containers run on the Docker daemon inside the sandbox, not on the host
Docker socket.

## Using the generated project

The following sections apply after either the macOS or Windows setup opens the
project in a VS Code Remote-SSH window.

### What is created

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

### Open the project again

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

### Working in the sandbox

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
Confirm that the app came from the
[GitHub Releases](https://github.com/mpg-age-bioinformatics/vscode-sandboxes/releases)
page before
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
