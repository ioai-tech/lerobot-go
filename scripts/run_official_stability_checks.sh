#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PY_PREFIX="${LEROBOT_OFFICIAL_PY_PREFIX:-$ROOT/.tmp/official-python/prefix/local}"
PY_SITE="${LEROBOT_OFFICIAL_PY_SITE:-$PY_PREFIX/lib/python3.12/dist-packages}"
PY_BIN="${LEROBOT_OFFICIAL_PY_BIN:-$PY_PREFIX/bin}"
DATA_ROOT="${LEROBOT_OFFICIAL_DATA_ROOT:-$ROOT/.tmp/official-datasets}"
WORK_ROOT="${LEROBOT_OFFICIAL_WORK_ROOT:-$ROOT/.tmp/official-checks}"
BIN="$WORK_ROOT/lerobot-go"

export PYTHONPATH="$PY_SITE${PYTHONPATH:+:$PYTHONPATH}"
export PATH="$PY_BIN:$PATH"

mkdir -p "$WORK_ROOT"
go build -o "$BIN" ./cmd/lerobot-go

python3 "$ROOT/scripts/official_lerobot_loader_check.py" \
  --repo-id "lerobot/pusht" \
  --root "$DATA_ROOT/lerobot-pusht" \
  --expect-version "v3.0" \
  --expect-episodes 206 \
  --expect-frames 25650

python3 "$ROOT/scripts/official_lerobot_loader_check.py" \
  --repo-id "lerobot/pusht_image" \
  --root "$DATA_ROOT/lerobot-pusht-image" \
  --expect-version "v3.0" \
  --expect-episodes 206 \
  --expect-frames 25650

if [ -f "$DATA_ROOT/libero-v3/libero_10/meta/info.json" ]; then
  python3 "$ROOT/scripts/official_lerobot_loader_check.py" \
    --repo-id "nvidia/LIBERO_LeRobot_v3" \
    --root "$DATA_ROOT/libero-v3/libero_10" \
    --expect-version "v3.0" \
    --expect-episodes 379 \
    --expect-frames 101469
fi

rm -rf "$WORK_ROOT/pusht_v21" "$WORK_ROOT/pusht_roundtrip_v30"
"$BIN" convert -i "$DATA_ROOT/lerobot-pusht" -o "$WORK_ROOT/pusht_v21" --to v2.1
"$BIN" convert -i "$WORK_ROOT/pusht_v21" -o "$WORK_ROOT/pusht_roundtrip_v30" --to v3.0
"$BIN" validate "$WORK_ROOT/pusht_roundtrip_v30"
python3 "$ROOT/scripts/official_lerobot_loader_check.py" \
  --repo-id "local/pusht_roundtrip_v30" \
  --root "$WORK_ROOT/pusht_roundtrip_v30" \
  --expect-version "v3.0" \
  --expect-episodes 206 \
  --expect-frames 25650

rm -rf "$WORK_ROOT/pusht_image_v21" "$WORK_ROOT/pusht_image_roundtrip_v30"
"$BIN" convert -i "$DATA_ROOT/lerobot-pusht-image" -o "$WORK_ROOT/pusht_image_v21" --to v2.1
"$BIN" convert -i "$WORK_ROOT/pusht_image_v21" -o "$WORK_ROOT/pusht_image_roundtrip_v30" --to v3.0
"$BIN" validate "$WORK_ROOT/pusht_image_roundtrip_v30"
python3 "$ROOT/scripts/official_lerobot_loader_check.py" \
  --repo-id "local/pusht_image_roundtrip_v30" \
  --root "$WORK_ROOT/pusht_image_roundtrip_v30" \
  --expect-version "v3.0" \
  --expect-episodes 206 \
  --expect-frames 25650

rm -rf "$WORK_ROOT/merge_src_a" "$WORK_ROOT/merge_src_b" "$WORK_ROOT/merged_pusht_image"
cp -a "$DATA_ROOT/lerobot-pusht-image" "$WORK_ROOT/merge_src_a"
cp -a "$DATA_ROOT/lerobot-pusht-image" "$WORK_ROOT/merge_src_b"
"$BIN" merge -o "$WORK_ROOT/merged_pusht_image" --to v3.0 -i "$WORK_ROOT/merge_src_a" -i "$WORK_ROOT/merge_src_b"
"$BIN" validate "$WORK_ROOT/merged_pusht_image"
python3 "$ROOT/scripts/official_lerobot_loader_check.py" \
  --repo-id "local/merged_pusht_image" \
  --root "$WORK_ROOT/merged_pusht_image" \
  --expect-version "v3.0" \
  --expect-episodes 412 \
  --expect-frames 51300

rm -rf "$WORK_ROOT/merge_video_a" "$WORK_ROOT/merge_video_b" "$WORK_ROOT/merged_pusht"
cp -a "$DATA_ROOT/lerobot-pusht" "$WORK_ROOT/merge_video_a"
cp -a "$DATA_ROOT/lerobot-pusht" "$WORK_ROOT/merge_video_b"
"$BIN" merge -o "$WORK_ROOT/merged_pusht" --to v3.0 -i "$WORK_ROOT/merge_video_a" -i "$WORK_ROOT/merge_video_b"
"$BIN" validate "$WORK_ROOT/merged_pusht"
python3 "$ROOT/scripts/official_lerobot_loader_check.py" \
  --repo-id "local/merged_pusht" \
  --root "$WORK_ROOT/merged_pusht" \
  --expect-version "v3.0" \
  --expect-episodes 412 \
  --expect-frames 51300
