#!/usr/bin/env bash
# scripts/check.sh — the verification gate every change must pass (local and CI).
# Usage: scripts/check.sh            (GO=/path/to/go to override the toolchain)
set -euo pipefail
cd "$(dirname "$0")/.."

GO="${GO:-go}"
if ! command -v "$GO" >/dev/null 2>&1 && [ -x "$HOME/.local/go/bin/go" ]; then
  GO="$HOME/.local/go/bin/go"
fi
GOFMT="$(dirname "$(command -v "$GO")")/gofmt"
[ -x "$GOFMT" ] || GOFMT=gofmt

step() { printf '\n== %s\n' "$*"; }

step "gofmt"
unformatted="$("$GOFMT" -l cmd internal evals web)"
if [ -n "$unformatted" ]; then echo "$unformatted"; echo "run: gofmt -w <files>"; exit 1; fi

step "go mod tidy -diff"
"$GO" mod tidy -diff

step "go build"
"$GO" build ./...

step "go vet"
"$GO" vet ./...

step "go test"
"$GO" test -count=1 ./...

step "node --check web/*.js"
if command -v node >/dev/null 2>&1; then
  for f in web/*.js; do node --check "$f"; done
else
  echo "node not found: skipped (JS syntax not checked)"
fi

printf '\nAll checks passed.\n'
