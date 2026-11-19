#!/usr/bin/env bash
# Runs as the language image's non-root development user.
set -euo pipefail
export PATH="$HOME/.local/bin:$PATH"
language="$(node -p 'require("./code/.sandbox/sandbox-runtime.json").language')"
bash code/.sandbox/install-coding-agents.sh
case "$language" in
  python)
    sudo chown "$(id -u):$(id -g)" .venv
    if [[ ! -x .venv/bin/python ]]; then python -m venv .venv; fi
    if [[ -f code/requirements.lock ]]; then .venv/bin/python -m pip install -r code/requirements.lock; fi
    ;;
  r)
    bash code/.sandbox/install-r-system-dependencies.sh
    sudo chown "$(id -u):$(id -g)" .r-library
    # Keep Bioconductor's R/version mapping available when this project reopens offline.
    if [[ ! -s code/.sandbox/bioconductor-config.yaml ]]; then
      curl --fail --location --retry 3 https://bioconductor.org/config.yaml -o code/.sandbox/bioconductor-config.yaml.tmp
      mv code/.sandbox/bioconductor-config.yaml.tmp code/.sandbox/bioconductor-config.yaml
    fi
    if ! Rscript code/.sandbox/verify-renv.R >/dev/null 2>&1; then Rscript code/.sandbox/setup-renv.R; fi
    # Also prepares sess once the VS Code R extension is present.
    Rscript --vanilla code/.sandbox/setup-vscode-r.R
    ;;
  *) echo 'Invalid language runtime.' >&2; exit 2 ;;
esac
