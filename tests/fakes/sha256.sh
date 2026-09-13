#!/usr/bin/env bash
# Prints the SHA-256 of a file, using whichever tool the platform ships.
set -euo pipefail

if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d" " -f1
else
    shasum -a 256 "$1" | cut -d" " -f1
fi
