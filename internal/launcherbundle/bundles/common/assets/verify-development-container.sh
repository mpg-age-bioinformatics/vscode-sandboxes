#!/usr/bin/env bash
set -euo pipefail
export PATH="$HOME/.local/bin:$PATH"
language="$(node -p 'require("./code/.sandbox/sandbox-runtime.json").language')"
series="$(node -p 'require("./code/.sandbox/sandbox-runtime.json").series')"
case "$language" in
  python)
    .venv/bin/python -c 'import sys; expected=tuple(map(int,sys.argv[1].split("."))); assert sys.version_info[:2]==expected,(sys.version,expected); print("Python development container:",sys.version)' "$series"
    ;;
  r)
    EXPECTED_R_SERIES="$series" Rscript -e 'actual <- paste(R.version$major, strsplit(R.version$minor, ".", fixed=TRUE)[[1]][1], sep="."); stopifnot(identical(actual, Sys.getenv("EXPECTED_R_SERIES"))); cat("R development container:", R.version.string, "\n")'
    Rscript code/.sandbox/verify-renv.R
    ;;
  *) exit 2 ;;
esac

# Check the real agent execution layer, without contacting a model or using a key.
echo 'Checking Codex command sandbox…'
if ! codex sandbox -- /bin/true; then
  echo 'Error: Codex cannot start its command sandbox. Rebuild the development container with the generated agent-sandbox-seccomp.json profile; do not disable Codex permissions.' >&2
  exit 1
fi
echo 'Codex command sandbox is working.'
