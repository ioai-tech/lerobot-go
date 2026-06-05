#!/usr/bin/env python3
"""Cross-validate LeRobot datasets (aligned with lerobot-go formatcheck)."""

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


def read_jsonl(path: Path) -> list[dict]:
    if not path.exists():
        return []
    rows = []
    for line in path.read_text().splitlines():
        line = line.strip()
        if line:
            rows.append(json.loads(line))
    return rows


def format_v21_path(template: str, episode_index: int, chunks_size: int) -> str:
    episode_chunk = episode_index // max(chunks_size, 1)
    out = template.replace("{episode_index:06d}", f"{episode_index:06d}")
    out = out.replace("{episode_chunk:03d}", f"{episode_chunk:03d}")
    return out


def v21_data_paths(root: Path, info: dict) -> list[str]:
    paths = []
    for ep in range(info.get("total_episodes", 0)):
        rel = format_v21_path(info["data_path"], ep, info.get("chunks_size", 1000))
        paths.append(rel)
    if len(paths) == 1 and not (root / paths[0]).exists():
        globbed = sorted((root / "data").glob("*/*.parquet"))
        if globbed:
            return [str(p.relative_to(root)) for p in globbed]
    return paths


def validate_v21(root: Path, info: dict, strict: bool) -> list[str]:
    errors: list[str] = []
    warnings: list[str] = []
    if info.get("codebase_version") != "v2.1":
        errors.append(f"codebase_version={info.get('codebase_version')}")
    for rel in (
        "meta/episodes.jsonl",
        "meta/tasks.jsonl",
        "meta/episodes_stats.jsonl",
    ):
        if not (root / rel).exists():
            errors.append(f"missing {rel}")
    if not (root / "meta/stats.json").exists():
        warnings.append("meta/stats.json missing (LeRobot load_stats returns None)")

    episodes = read_jsonl(root / "meta/episodes.jsonl")
    if len(episodes) != info.get("total_episodes", -1):
        errors.append(f"episodes.jsonl count {len(episodes)} != total_episodes")

    stats_rows = read_jsonl(root / "meta/episodes_stats.jsonl")
    for row in stats_rows:
        stats = row.get("stats")
        if not isinstance(stats, dict):
            errors.append("episodes_stats.jsonl: stats must be feature dict")
            break

    feature_cols = [
        k for k, v in info.get("features", {}).items() if v.get("dtype") not in ("video", "image")
    ]

    total_frames = 0
    global_index = 0
    data_paths = v21_data_paths(root, info)
    if len(data_paths) != info.get("total_episodes", 0):
        errors.append(f"data parquet files {len(data_paths)} != total_episodes")

    for ep, rel in enumerate(data_paths):
        pq_path = root / rel
        if not pq_path.exists():
            errors.append(f"missing {rel}")
            continue
        table = pq.read_table(pq_path)
        total_frames += table.num_rows
        names = {field.name for field in table.schema}
        for col in ("timestamp", "frame_index", "episode_index", "index", "task_index"):
            if col not in names:
                errors.append(f"{pq_path.name}: missing column {col}")
        for col in feature_cols:
            if col not in names:
                errors.append(f"{rel}: missing feature column {col}")
        index_col = table.column("index").to_pylist()
        if index_col and index_col[0] != global_index:
            errors.append(f"episode {ep}: index starts at {index_col[0]}, want {global_index}")
        global_index += table.num_rows

    if total_frames != info.get("total_frames"):
        errors.append(f"parquet frames {total_frames} != total_frames")

    want_split = f"0:{info.get('total_episodes')}"
    if info.get("splits", {}).get("train") != want_split:
        errors.append(f"splits.train want {want_split}")

    if strict and info.get("video_path"):
        for ep in range(info.get("total_episodes", 0)):
            for key, spec in info.get("features", {}).items():
                if spec.get("dtype") != "video":
                    continue
                rel = format_v21_path(info["video_path"], ep, info.get("chunks_size", 1000))
                rel = rel.replace("{video_key}", key)
                if not (root / rel).exists():
                    errors.append(f"episode {ep}: missing video {rel}")

    return errors


def validate_v30(root: Path, info: dict, strict: bool) -> list[str]:
    errors: list[str] = []
    if info.get("codebase_version") != "v3.0":
        errors.append(f"codebase_version={info.get('codebase_version')}")
    for rel in ("meta/tasks.parquet",):
        if not (root / rel).exists():
            errors.append(f"missing {rel}")
    if not (root / "meta/stats.json").exists():
        pass  # optional in official loader

    feature_cols = [
        k for k, v in info.get("features", {}).items() if v.get("dtype") not in ("video", "image")
    ]

    data_files = sorted((root / "data").rglob("*.parquet"))
    if not data_files:
        errors.append("missing data parquet files")
    total_rows = 0
    expect_index = 0
    for path in data_files:
        table = pq.read_table(path)
        total_rows += table.num_rows
        names = {field.name for field in table.schema}
        for col in ("timestamp", "frame_index", "episode_index", "index", "task_index"):
            if col not in names:
                errors.append(f"{path.name}: missing column {col}")
        for col in feature_cols:
            if col not in names:
                errors.append(f"{path.relative_to(root)}: missing feature column {col}")
        index_col = table.column("index").to_pylist()
        for i, idx in enumerate(index_col):
            if idx != expect_index + i:
                errors.append(f"{path.name}: global index[{i}]={idx} want {expect_index + i}")
                break
        expect_index += table.num_rows

    if total_rows != info.get("total_frames"):
        errors.append(f"data rows {total_rows} != total_frames")

    ep_files = sorted((root / "meta" / "episodes").rglob("*.parquet"))
    if not ep_files:
        errors.append("missing meta/episodes parquet")
    ep_rows = sum(pq.read_metadata(p).num_rows for p in ep_files)
    if ep_rows != info.get("total_episodes"):
        errors.append(f"episodes meta rows {ep_rows} != total_episodes")

    tasks_path = root / "meta" / "tasks.parquet"
    if tasks_path.exists():
        tasks = pq.read_table(tasks_path).to_pandas()
        if len(tasks) != info.get("total_tasks"):
            errors.append(f"tasks rows {len(tasks)} != total_tasks")

    want_split = f"0:{info.get('total_episodes')}"
    if info.get("splits", {}).get("train") != want_split:
        errors.append(f"splits.train want {want_split}")

    return errors


def print_tree(root: Path) -> None:
    files = sorted(p.relative_to(root) for p in root.rglob("*") if p.is_file())
    print(f"\n{root} ({len(files)} files):")
    for rel in files:
        print(f"  {rel}")


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("root", type=Path)
    p.add_argument("--tree", action="store_true", help="print file tree")
    p.add_argument("--strict", action="store_true", default=True)
    p.add_argument("--no-strict", action="store_false", dest="strict")
    args = p.parse_args()

    root = args.root.resolve()
    info = load_info(root)
    version = info.get("codebase_version")
    if version == "v2.1":
        errors = validate_v21(root, info, args.strict)
    elif version == "v3.0":
        errors = validate_v30(root, info, args.strict)
    else:
        errors = [f"unsupported version {version}"]

    if args.tree:
        print_tree(root)

    print(f"version: {version}")
    print(f"episodes: {info.get('total_episodes')} frames: {info.get('total_frames')} tasks: {info.get('total_tasks')}")

    if errors:
        for e in errors:
            print("ERROR:", e)
        return 1
    print("OK: dataset format valid")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
