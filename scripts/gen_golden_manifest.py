#!/usr/bin/env python3
"""Generate golden manifest JSON from a LeRobot dataset root."""

from __future__ import annotations

import argparse
import json
from pathlib import Path


def main() -> None:
    p = argparse.ArgumentParser()
    p.add_argument("root", type=Path)
    p.add_argument("-o", "--output", type=Path, required=True)
    args = p.parse_args()

    info_path = args.root / "meta" / "info.json"
    manifest = {
        "root": str(args.root),
        "info": json.loads(info_path.read_text()) if info_path.exists() else None,
        "files": sorted(str(p.relative_to(args.root)) for p in args.root.rglob("*") if p.is_file()),
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(manifest, indent=2))
    print(f"wrote {args.output}")


if __name__ == "__main__":
    main()
