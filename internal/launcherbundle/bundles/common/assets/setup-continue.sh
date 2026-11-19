#!/usr/bin/env bash
# Used by the general bioinformatics sandbox; language containers use the same
# preparation and installation helpers through the Dev Containers CLI.
set -euo pipefail
bundle="$(python3 code/.sandbox/prepare-continue.py)"
if node code/.sandbox/install-continue.js check; then exit 0; fi
tar -C "$(dirname -- "$bundle")" -cf - "$(basename -- "$bundle")" | node code/.sandbox/install-continue.js install
