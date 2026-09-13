#!/usr/bin/env bash
# Reviews the content it was actually given, and refuses to report on anything
# else.
#
# This is the property fg-64y.37 exists for: a provider handed only a target
# identity cannot see a pasted argument or a frozen snapshot at all, and a
# finding it reported would be about nothing. The digest check is the proof, so
# it runs here where both values are hex.
set -euo pipefail

FAKES="$(cd "$(dirname "$0")" && pwd)"

if [[ -z "${GRIMES_TARGET_CONTENT:-}" ]]; then
    echo "provider was given no content to review" >&2
    exit 1
fi
if [[ ! -e "$GRIMES_TARGET_CONTENT" ]]; then
    echo "content path ${GRIMES_TARGET_CONTENT} does not exist" >&2
    exit 1
fi

# A code target is a tree, and its fingerprint is taken over path-and-digest
# pairs rather than any one file's bytes, so only the single-file kinds compare.
if [[ "${GRIMES_TARGET_KIND:-code}" != "code" ]]; then
    if [[ -d "$GRIMES_TARGET_CONTENT" ]]; then
        echo "content for ${GRIMES_TARGET_KIND} is a directory" >&2
        exit 1
    fi
    if command -v sha256sum >/dev/null 2>&1; then
        GOT="$(sha256sum "$GRIMES_TARGET_CONTENT" | cut -d" " -f1)"
    else
        GOT="$(shasum -a 256 "$GRIMES_TARGET_CONTENT" | cut -d" " -f1)"
    fi
    if [[ "$GOT" != "${GRIMES_TARGET_FINGERPRINT:-}" ]]; then
        echo "content digest ${GOT} is not the target ${GRIMES_TARGET_FINGERPRINT:-(unset)}" >&2
        exit 1
    fi
elif [[ ! -d "$GRIMES_TARGET_CONTENT" ]]; then
    echo "content for a code target is not a tree" >&2
    exit 1
fi

cd "$(mktemp -d)"
echo "I read the target I was given."
# shellcheck disable=SC2016  # the quoted text is evidence, not an expansion
"$FAKES/add-finding.sh" P0 systemic likely \
    "caller-controlled deletion path" 'rm -rf "$1"/*'
"$FAKES/seal.sh" 6 5 "The content I was handed is the target the engine named."
