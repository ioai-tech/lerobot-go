#!/usr/bin/env bash
# Write sample v2.1 and v3.0 datasets and validate format.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${LEROBOT_E2E_OUT:-$ROOT/testdata/output}"

cd "$ROOT"
echo "==> Go tests (write datasets to $OUT)"
go test ./lerobot -run 'TestWriteDatasetFormats|TestCLI' -count=1 -v
go test ./internal/v21 ./internal/v30 ./internal/formatcheck ./internal/cli -count=1

echo "==> lerobot-go CLI validate"
go build -o "$ROOT/lerobot-go" ./cmd/lerobot-go
"$ROOT/lerobot-go" validate "$OUT/v21"
"$ROOT/lerobot-go" validate "$OUT/v30"

if command -v python3 >/dev/null 2>&1; then
  if python3 -c "import pyarrow" 2>/dev/null; then
    echo "==> Python validation"
    python3 "$ROOT/scripts/validate_dataset.py" "$OUT/v21" --tree
    python3 "$ROOT/scripts/validate_dataset.py" "$OUT/v30" --tree
  else
    echo "skip python validation (pip install pyarrow)"
  fi
else
  echo "skip python validation (python3 not found)"
fi

echo ""
echo "Done. Inspect datasets:"
echo "  v2.1: $OUT/v21"
echo "  v3.0: $OUT/v30"
