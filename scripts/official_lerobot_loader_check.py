#!/usr/bin/env python3
"""Load a dataset with the official lerobot package and sanity-check access."""

from __future__ import annotations

import argparse
import json
from pathlib import Path

from lerobot.datasets.lerobot_dataset import LeRobotDataset


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo-id", required=True, help="Repo id label passed to LeRobotDataset")
    parser.add_argument("--root", required=True, help="Local dataset root")
    parser.add_argument("--expect-episodes", type=int)
    parser.add_argument("--expect-frames", type=int)
    parser.add_argument("--expect-version")
    parser.add_argument("--sample-index", type=int, default=0)
    parser.add_argument("--video-backend", default="pyav")
    args = parser.parse_args()

    root = Path(args.root).resolve()
    dataset = LeRobotDataset(
        repo_id=args.repo_id,
        root=root,
        download_videos=False,
        video_backend=args.video_backend,
    )

    info = json.loads((root / "meta" / "info.json").read_text())
    if args.expect_episodes is not None and dataset.num_episodes != args.expect_episodes:
        raise SystemExit(f"num_episodes={dataset.num_episodes} want {args.expect_episodes}")
    if args.expect_frames is not None and dataset.num_frames != args.expect_frames:
        raise SystemExit(f"num_frames={dataset.num_frames} want {args.expect_frames}")
    if args.expect_version is not None and info.get("codebase_version") != args.expect_version:
        raise SystemExit(f"codebase_version={info.get('codebase_version')} want {args.expect_version}")

    sample = dataset[args.sample_index]
    print(
        json.dumps(
            {
                "repo_id": args.repo_id,
                "root": str(root),
                "codebase_version": info.get("codebase_version"),
                "num_episodes": dataset.num_episodes,
                "num_frames": dataset.num_frames,
                "sample_index": args.sample_index,
                "sample_keys": sorted(sample.keys()),
            },
            indent=2,
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
