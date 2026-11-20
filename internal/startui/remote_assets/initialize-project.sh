#!/usr/bin/env bash
# Runs as the HPC user inside Apptainer; no package operations on the login node.
set -euo pipefail
cd -- "${1:-/workspace}"
run=/run/vscode-launcher
[[ -f "$run/project.tar" ]] || exit 0
python3 - "$run/project.tar" <<'PY'
import json,os,pathlib,tarfile,sys
root=pathlib.Path.cwd()
for name in ['code/.sandbox','data','.vscode']:
    target=root/name
    if target.is_symlink() or (root!=target.resolve() and root not in target.resolve().parents): raise RuntimeError('Refusing a project directory outside the workspace: '+name)
    target.mkdir(parents=True,exist_ok=True)
with tarfile.open(sys.argv[1]) as archive:
    expected=json.loads(archive.extractfile('code/.sandbox/remote-project.json').read())
    marker=root/'code/.sandbox/remote-project.json'
    if marker.exists() and json.loads(marker.read_bytes())!=expected:
        raise RuntimeError('Existing remote project has a different preset/version/architecture; use another project folder')
    for member in archive:
        name=pathlib.PurePosixPath(member.name)
        if name.is_absolute() or '..' in name.parts or not member.isfile(): raise RuntimeError('Invalid project archive entry')
        dest=root/str(name)
        # Every ancestor must stay within the workspace, including existing links.
        if root!=dest.parent.resolve() and root not in dest.parent.resolve().parents: raise RuntimeError('Project path escapes workspace')
        if dest.is_symlink(): raise RuntimeError('Refusing symlinked project file: '+str(name))
        content=archive.extractfile(member).read()
        if dest.exists():
            old=dest.read_bytes()
            if old==content: continue
            if str(name)=='code/.sandbox/remote-project.json':
                if json.loads(old)!=json.loads(content): raise RuntimeError('Existing remote project has a different preset/version/architecture; use another project folder')
            elif str(name)=='code/Dockerfile':
                dest=root/'code/Dockerfile.remote'
                # This is launcher-owned recipe output; preserve edited versions.
                if dest.exists() and dest.read_bytes()!=content: raise RuntimeError('Dockerfile.remote differs; preserve or rename it before rebuilding')
            elif str(name)=='AGENTS.md':
                remote=content.decode().split('## Remote execution',1)[1]
                if '## Remote execution' in old.decode(): continue
                content=old+b'\n\n## Remote execution (Apptainer session)\nWhen VSCODE_SANDBOX_REMOTE_CONTAINER is set, these instructions take precedence over local Docker runtime instructions.\n'+remote.encode()
            elif str(name)=='.gitignore':
                rules=old.decode().splitlines();extra=[line for line in content.decode().splitlines() if line and line not in rules]
                content=old+b'\n'+('\n'.join(extra)+'\ndata*/\n').encode()
            elif str(name)=='code/.sandbox/remote-image.lock.json':
                if json.loads(old)==json.loads(content): continue
                raise RuntimeError('Existing image lock differs; use a new folder or deliberately archive the previous remote image metadata')
            else:
                print('Keeping existing project file:',name);continue
        dest.parent.mkdir(parents=True,exist_ok=True)
        # Exclusive creation prevents racing a concurrent user edit on new files.
        mode='wb' if str(name) in ['.gitignore','AGENTS.md'] and dest.exists() else 'xb'
        with dest.open(mode) as output: output.write(content)
        dest.chmod(member.mode & 0o777)
PY
source_type="$(python3 -c 'import json;print(json.load(open("code/.sandbox/remote-project.json"))["source"])')"
export PATH="$HOME/.local/bin:/usr/local/bin:$PATH"
if [[ "$source_type" == python || "$source_type" == r ]];then
 [[ ! -L code/INSTALL.md ]] || { echo 'Refusing a symlinked installation history.' >&2; exit 1; }
 printf '\n## Remote startup — %s\n\nImage: code/.sandbox/remote-image.lock.json.\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> code/INSTALL.md
 trap 'status=$?; printf "\nInstallation failed (exit %s); inspect the Slurm log before retrying.\n" "$status" >> code/INSTALL.md; exit "$status"' ERR
fi
if [[ "${VSCODE_SANDBOX_NETWORK:-open}" == none ]]; then
 echo 'No internet: keeping prepared project packages; online package resolution and skills cloning are skipped.'
 if [[ "$source_type" == python ]]; then
  [[ -x .venv/bin/python ]] || python3 -m venv .venv
  .venv/bin/python -m pip check
 elif [[ "$source_type" == r ]]; then
  mkdir -p .r-library
 fi
 printf '\nNo internet selected: online dependency/skills installation skipped. Prepare locked dependencies with Selected sites or bake them into the image before using offline mode.\n' >> code/INSTALL.md
 exit 0
fi
case "$source_type" in
 python)
  printf '%s\n' 'Preparing Python environment and locked packages' > "$run/runtime/startup-phase"
  if [[ ! -x .venv/bin/python ]]; then python3 -m venv .venv;fi
  # A previous allocation mounted this same project at /workspace. Refresh
  # virtualenv activation and locked entry points when adopting the exact path.
  install_args=()
  if [[ "$PWD" != /workspace && -f .venv/bin/activate ]] && grep -Fq '/workspace/.venv' .venv/bin/activate; then
   python3 - <<'PYVENV'
import os,venv
builder=venv.EnvBuilder()
builder.setup_scripts(builder.ensure_directories(os.path.abspath('.venv')))
PYVENV
   install_args=(--force-reinstall)
   printf '\nRelocated Python environment from the legacy /workspace mount to the exact HPC project path; refreshed virtualenv and reinstalled locked package entry points.\n' >> code/INSTALL.md
  fi
  # The same lockfile conventions as Local, with notebook tools recorded.
  if [[ ! -f code/requirements.lock ]]; then
   if [[ ! -f code/requirements.in ]];then printf 'ipykernel==6.30.1\njupytext==1.17.3\npip==25.2\npip-tools==7.5.1\nclick==8.2.1\n' > code/requirements.in;fi
   .venv/bin/python -m pip install 'pip==25.2' 'pip-tools==7.5.1' 'click==8.2.1'
   .venv/bin/pip-compile --allow-unsafe --generate-hashes --output-file code/requirements.lock code/requirements.in
  fi
  .venv/bin/python -m pip install "${install_args[@]}" -r code/requirements.lock
  .venv/bin/python -m pip check
  printf '\nPrepared .venv; resolved code/requirements.in with pip==25.2, pip-tools==7.5.1 and click==8.2.1 when no lock existed, installed with `.venv/bin/python -m pip install -r code/requirements.lock`, and verified with pip check. Exact installed dependency versions are recorded in code/requirements.lock.\n' >> code/INSTALL.md
  ;;
 r)
  printf '%s\n' 'Preparing R environment and locked packages' > "$run/runtime/startup-phase"
  mkdir -p .r-library
  if [[ ! -s code/.sandbox/bioconductor-config.yaml ]]; then
   curl --fail --location --retry 3 https://bioconductor.org/config.yaml -o code/.sandbox/bioconductor-config.yaml.tmp
   mv code/.sandbox/bioconductor-config.yaml.tmp code/.sandbox/bioconductor-config.yaml
  fi
  if ! Rscript --vanilla code/.sandbox/verify-remote-renv.R >/dev/null 2>&1;then Rscript --vanilla code/.sandbox/setup-remote-renv.R;fi
  Rscript --vanilla code/.sandbox/setup-vscode-r.R
  # Record the baked OS libraries using the same local installation manifest.
  bash code/.sandbox/install-r-system-dependencies.sh
  printf '\nRan `Rscript --vanilla code/.sandbox/setup-remote-renv.R` when restore/verification was needed, `Rscript --vanilla code/.sandbox/setup-vscode-r.R`, and the shared OS-library verification script. Exact R and OS versions are recorded in renv.lock and code/.sandbox/r-system-packages.lock.\n' >> code/INSTALL.md
  ;;
esac
if [[ "$source_type" == python || "$source_type" == r || "$source_type" == bioinformatics ]];then
 printf '%s\n' 'Preparing project skills and Git repository' > "$run/runtime/startup-phase"
 source code/.sandbox/setup-skills.sh
 setup_sandbox_skills https://github.com/mpg-age-bioinformatics/skills.git
 # Git operations run only inside the HPC container; existing staging is kept.
 if [[ ! -e .git ]];then
  git init
  git config user.name 'Sandbox Launcher';git config user.email 'sandbox@localhost'
  mkdir -p .git/hooks
  cp code/.sandbox/pre-commit .git/hooks/pre-commit;chmod +x .git/hooks/pre-commit
  git add -A
  git commit -m 'Initialize remote sandbox project'
 fi
fi
