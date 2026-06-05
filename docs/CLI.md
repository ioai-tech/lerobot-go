# CLI reference

Install: `go install github.com/ioai-tech/lerobot-go/cmd/lerobot-go@latest`

```text
lerobot-go [global flags] <command> [command flags] [args]
```

## Global flags

| Flag | Default | Description |
|------|---------|-------------|
| `-v`, `--verbose` | `false` | Log details to stderr |
| `--ffmpeg-path` | — | Path to `ffmpeg` (overrides `PATH`) |
| `--json` | `false` | JSON output on stdout (`validate`, `create`, `merge`, …) |

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Command failed (validation, I/O, merge error) |
| `2` | Usage / argument error |

## `validate`

Check that a directory matches LeRobot v2.1 or v3.0 on-disk layout.

```bash
lerobot-go validate ./dataset
lerobot-go validate ./dataset --json
lerobot-go validate ./dataset --tree -v
```

| Flag | Default | Description |
|------|---------|-------------|
| `--strict` | `true` | Full formatcheck when `data/` exists |
| `--tree` | `false` | Print file tree (human-readable, stderr) |

## `convert`

Convert between v2.1 and v3.0. Writes to a new `--output` directory (no in-place edit).

```bash
lerobot-go convert -i ./dataset_v21 -o ./dataset_v30 --to v3.0
lerobot-go convert -i ./dataset_v30 -o ./dataset_v21 --to v2.1
```

| Flag | Default | Description |
|------|---------|-------------|
| `-i`, `--input` | (required) | Source dataset root |
| `-o`, `--output` | (required) | Destination root |
| `--to` | (required) | Target version: `v2.1` or `v3.0` |
| `--from` | auto | Source version from `meta/info.json` |
| `--force` | `false` | Allow non-empty output directory |
| `--data-file-size-mb` | `100` | v3.0 data chunk rotation (MB) |
| `--video-file-size-mb` | `200` | v3.0 video chunk size (MB) |
| `--chunks-size` | `1000` | Max files per chunk |

## `create`

Finalize completed staging episodes (`ep_*`) into the official dataset layout. Equivalent to calling `lerobot.Merge` from the Go API.

Use after parallel episode writers finish under `<output>/_staging` (or `--staging`).

```bash
lerobot-go create -o ./dataset --version v3.0 --fps 30 --features ./features.json
lerobot-go create -o ./dataset --version v2.1 --fps 10 --features ./features.json --stats-mode full
lerobot-go create -o ./dataset --version v3.0 --fps 30 --features ./features.json \
  --staging /path/to/staging --force
```

| Flag | Default | Description |
|------|---------|-------------|
| `-o`, `--output` | (required) | Dataset root (final layout) |
| `--version` | (required) | `v2.1` or `v3.0` |
| `--features` | (required) | JSON map of feature name → `{dtype, shape}` |
| `--fps` | (required) | Dataset FPS |
| `--staging` | `<output>/_staging` | Staging root with `ep_*` dirs |
| `--robot-type` | — | Stored in `meta/info.json` |
| `--stats-mode` | `sampled` | `sampled` or `full` for image/video stats |
| `--force` | `false` | Allow non-empty output directory |

### Features JSON example

```json
{
  "observation.state": { "dtype": "float32", "shape": [7] },
  "action": { "dtype": "float32", "shape": [7] },
  "observation.images.cam": { "dtype": "video", "shape": [480, 640, 3] }
}
```

Use `"dtype": "image"` instead of `"video"` for parquet-embedded images (v2.1-img).

## `merge`

Merge two or more **completed** datasets into one output dataset.

```bash
lerobot-go merge -o ./merged --to v3.0 -i ./a -i ./b
lerobot-go merge -o ./merged_v21 --to v2.1 ./ds1 ./ds2 ./ds3
```

| Flag | Default | Description |
|------|---------|-------------|
| `-o`, `--output` | (required) | Output dataset root |
| `--to` | (required) | Output version: `v2.1` or `v3.0` |
| `-i`, `--input` | — | Source path (repeatable); positional args also accepted |
| `--force` | `false` | Allow non-empty output directory |
| `--data-file-size-mb` | `100` | v3.0 data chunk limit |
| `--video-file-size-mb` | `200` | v3.0 video chunk limit |
| `--chunks-size` | `1000` | Max files per chunk |
| `--stats-mode` | `sampled` | Used when recomputing media stats; source stats are kept when possible |

## `version`

```bash
lerobot-go version
```

## `completion`

```bash
lerobot-go completion bash > /etc/bash_completion.d/lerobot-go
lerobot-go completion zsh > "${fpath[1]}/_lerobot-go"
```

Supported shells: `bash`, `zsh`, `fish`, `powershell`.
