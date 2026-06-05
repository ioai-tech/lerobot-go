# LeRobot v2.1 Write Protocol (Go reference)

## Layout

- `meta/info.json` — `codebase_version: "v2.1"`
- `meta/episodes.jsonl`, `meta/tasks.jsonl`, `meta/episodes_stats.jsonl`
- `data/chunk-000/episode_XXXXXX.parquet`
- `videos/chunk-000/{camera}/episode_XXXXXX.mp4`

## Differences from v3.0

- One parquet + one mp4 per episode (easier parallel write)
- JSONL metadata instead of parquet episodes table
- `add_frame(frame, task)` — task as separate argument in Python; Go `Frame.Task` field

## Merge

Staging episodes copy to final paths; jsonl files appended; global stats aggregated.
