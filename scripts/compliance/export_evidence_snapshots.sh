#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT_DIR="$ROOT_DIR/docs/generated"
WORKTREE_DIR="${GRXFIRMA_EVIDENCE_WORKTREE:-/tmp/grxfirma-evidence}"
HEAD_REV="$(git -C "$ROOT_DIR" rev-parse HEAD)"

export GOCACHE="${GOCACHE:-/tmp/grxfirma-gocache}"
export GOMODCACHE="${GOMODCACHE:-$(go env GOMODCACHE 2>/dev/null || printf '%s/go/pkg/mod' "$HOME")}"

mkdir -p "$OUT_DIR"
rm -rf "$WORKTREE_DIR"
git -C "$ROOT_DIR" worktree prune >/dev/null 2>&1 || true

cleanup() {
  git -C "$ROOT_DIR" worktree remove --force "$WORKTREE_DIR" >/dev/null 2>&1 || true
}
trap cleanup EXIT

git -C "$ROOT_DIR" worktree add --detach "$WORKTREE_DIR" "$HEAD_REV" >/dev/null

run_and_capture() {
  local outfile="$1"
  shift
  {
    printf 'fecha=%s\n' "$(date -Iseconds)"
    printf 'head=%s\n' "$HEAD_REV"
    printf 'cwd=%s\n' "$WORKTREE_DIR"
    printf 'cmd='
    printf '%q ' "$@"
    printf '\n\n'
    (
      cd "$WORKTREE_DIR"
      "$@"
    )
  } >"$outfile" 2>&1
}

echo "Generando snapshots versionados en: $OUT_DIR"

run_and_capture "$OUT_DIR/evidence-build.txt" go build ./...
run_and_capture "$OUT_DIR/evidence-regression.txt" go test ./test/regression/... -run 'Test(PAdES|XAdES|CAdES)_V1' -v
run_and_capture "$OUT_DIR/evidence-conformance.txt" go test ./test/conformance/... -v

cat <<EOF
Snapshots actualizados:
  - $OUT_DIR/evidence-build.txt
  - $OUT_DIR/evidence-regression.txt
  - $OUT_DIR/evidence-conformance.txt

Notas:
  - Se ha usado un worktree limpio en $WORKTREE_DIR
  - HEAD evaluado: $HEAD_REV
  - GOCACHE: $GOCACHE
  - GOMODCACHE: $GOMODCACHE
EOF
