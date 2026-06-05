# LeRobot v3.0 Write Protocol (Go reference)

## Layout

- `meta/info.json` — features schema, counters, path templates
- `meta/stats.json` — aggregated normalization stats
- `meta/tasks.parquet` — task index mapping
- `meta/episodes/chunk-*/file-*.parquet` — per-episode metadata
- `data/chunk-*/file-*.parquet` — frame data (snappy, dictionary)
- `videos/{video_key}/chunk-*/file-*.mp4` — encoded camera streams

## Auto-injected columns

`timestamp`, `frame_index`, `episode_index`, `index`, `task_index`

## Chunk rotation

- Data parquet: rotate when **uncompressed** size estimate exceeds `data_files_size_in_mb` (default 100)
- Video: rotate when file size exceeds `video_files_size_in_mb` (default 200)
- Use `UpdateChunkFileIndices(chunk, file, chunks_size)` when `file == chunks_size-1`

## Parquet writer settings (match Python)

```python
pq.ParquetWriter(path, schema=table.schema, compression="snappy", use_dictionary=True)
```

Go uses `apache/arrow-go/v18` with `parquet.WithCompression(Snappy)` and `WithDictionaryDefault(true)`.

## Parallel write pattern

1. Each episode → isolated `staging/ep_XXXXXX/` with `frames.parquet`, `videos/*.mp4`, `episode_meta.json`
2. Merger sorts by `episode_index`, assigns global `index`, appends parquet, concat MP4 with DTS-safe ffmpeg
