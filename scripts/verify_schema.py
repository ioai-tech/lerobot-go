#!/usr/bin/env python3
"""Strict schema verification between golden and candidate LeRobot datasets."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

try:
    import pyarrow.parquet as pq
except ImportError:
    print("pyarrow required: pip install pyarrow", file=sys.stderr)
    sys.exit(2)


def load_info(root: Path) -> dict:
    with open(root / "meta" / "info.json") as f:
        return json.load(f)


def parquet_files(root: Path) -> list[Path]:
    data = root / "data"
    if not data.exists():
        return []
    return sorted(data.rglob("*.parquet"))


def schema_signature(path: Path) -> dict:
    schema = pq.read_schema(path)
    cols = []
    for field in schema:
        cols.append({
            "name": field.name,
            "type": str(field.type),
            "nullable": field.nullable,
        })
    meta = pq.read_metadata(path)
    return {
        "path": str(path),
        "columns": cols,
        "num_rows": meta.num_rows,
        "serialized_size": meta.serialized_size,
    }


def compare_features(golden: dict, candidate: dict) -> list[str]:
    diffs: list[str] = []
    gf = golden.get("features", {})
    cf = candidate.get("features", {})
    for k in gf:
        if k not in cf:
            diffs.append(f"missing feature {k}")
            continue
        if gf[k].get("dtype") != cf[k].get("dtype"):
            diffs.append(f"{k} dtype {gf[k].get('dtype')} vs {cf[k].get('dtype')}")
        if gf[k].get("shape") != cf[k].get("shape"):
            diffs.append(f"{k} shape {gf[k].get('shape')} vs {cf[k].get('shape')}")
    for k in cf:
        if k not in gf:
            diffs.append(f"extra feature {k}")
    return diffs


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("--golden", type=Path, required=True)
    p.add_argument("--candidate", type=Path, required=True)
    args = p.parse_args()

    ginfo = load_info(args.golden)
    cinfo = load_info(args.candidate)
    diffs = compare_features(ginfo, cinfo)
    if ginfo.get("fps") != cinfo.get("fps"):
        diffs.append(f"fps {ginfo.get('fps')} vs {cinfo.get('fps')}")

    gfiles = parquet_files(args.golden)
    cfiles = parquet_files(args.candidate)
    if len(gfiles) != len(cfiles):
        diffs.append(f"parquet file count {len(gfiles)} vs {len(cfiles)}")
    for gp, cp in zip(gfiles, cfiles):
        gs = schema_signature(gp)
        cs = schema_signature(cp)
        if gs["columns"] != cs["columns"]:
            diffs.append(f"column schema mismatch: {gp.name} vs {cp.name}")

    if diffs:
        for d in diffs:
            print("DIFF:", d)
        return 1
    print("OK: strict schema checks passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
