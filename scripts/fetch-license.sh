#!/usr/bin/env bash
#
# fetch-license.sh — install the verbatim GNU AGPL-3.0 text as ./LICENSE.
#
# The LICENSE file MUST be the exact FSF text. This script downloads it from the
# canonical source and verifies its integrity BEFORE writing, so the result is
# reproducible and free of transcription lapses. It fails closed: if the download
# does not look like the genuine, complete AGPL-3.0, nothing is written.
#
# Usage:  ./scripts/fetch-license.sh
#
set -euo pipefail

URL="https://www.gnu.org/licenses/agpl-3.0.txt"

# Resolve repo root (git if available, else the script's parent dir).
if ROOT="$(git rev-parse --show-toplevel 2>/dev/null)"; then :; else
  ROOT="$(cd "$(dirname "$0")/.." && pwd)"
fi
DEST="$ROOT/LICENSE"

TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT

echo "Fetching canonical AGPL-3.0 from $URL ..."
curl -fsSL --proto '=https' --tlsv1.2 "$URL" -o "$TMP"

# --- integrity verification (fail closed) ---
# Anchors are punctuation-independent substrings that appear verbatim in the
# canonical AGPL-3.0. "Remote Network Interaction" is the AGPL-distinguishing
# section (absent from the plain GPL).
need() { grep -qF -- "$1" "$TMP" || { echo "VERIFY FAILED: missing \"$1\"" >&2; exit 1; }; }
need "GNU AFFERO GENERAL PUBLIC LICENSE"
need "Version 3, 19 November 2007"
need "Preamble"
need "TERMS AND CONDITIONS"
need "Remote Network Interaction"
need "Disclaimer of Warranty"
need "Limitation of Liability"
need "Interpretation of Sections 15 and 16"
need "How to Apply These Terms to Your New Programs"
need "gnu.org/licenses"

bytes="$(wc -c < "$TMP" | tr -d '[:space:]')"
if [ "$bytes" -lt 30000 ] || [ "$bytes" -gt 40000 ]; then
  echo "VERIFY FAILED: unexpected size ${bytes} bytes (expected ~34.5 KB)" >&2
  exit 1
fi

mv "$TMP" "$DEST"
trap - EXIT
echo "OK: wrote verbatim AGPL-3.0 to LICENSE (${bytes} bytes, all anchors present)."
echo "Next: git add LICENSE && git commit -m 'Add verbatim AGPL-3.0 license text'"
