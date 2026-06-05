# lerobot-go

[中文](README_zh.md) · Maintained by [IO-AI.TECH](https://io-ai.tech)

[![CI](https://github.com/ioai-tech/lerobot-go/actions/workflows/ci.yml/badge.svg)](https://github.com/ioai-tech/lerobot-go/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/ioai-tech/lerobot-go)](https://goreportcard.com/report/github.com/ioai-tech/lerobot-go)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/ioai-tech/lerobot-go)](https://github.com/ioai-tech/lerobot-go/releases)

Go library and CLI for [LeRobot](https://github.com/huggingface/lerobot) datasets (v2.1 / v3.0): write, validate, convert, and merge. On-disk layout matches the official HuggingFace format.

**Requirements:** Go 1.26+; `ffmpeg` / `ffprobe` on `PATH` for video features.

## Install

```bash
go install github.com/ioai-tech/lerobot-go/cmd/lerobot-go@latest
```

Pre-built binaries: [Releases](https://github.com/ioai-tech/lerobot-go/releases) (`linux` / `darwin` / `windows` × `amd64` / `arm64`, with `checksums.txt`).

## CLI

```bash
lerobot-go validate ./dataset
lerobot-go convert -i ./v21 -o ./v30 --to v3.0
lerobot-go create -o ./dataset --version v3.0 --fps 30 --features ./features.json
lerobot-go merge -o ./merged --to v3.0 -i ./a -i ./b
```

| Command | Description |
|---------|-------------|
| `validate` | Check v2.1 / v3.0 on-disk layout |
| `convert` | Convert between v2.1 and v3.0 |
| `create` | Finalize `_staging/ep_*` into a dataset |
| `merge` | Combine multiple datasets |
| `version` / `completion` | Version info and shell completion |

Global flags: `-v`, `--ffmpeg-path`, `--json`. Full reference: [docs/CLI.md](docs/CLI.md).

## Library

```go
import "github.com/ioai-tech/lerobot-go/lerobot"
```

Typical flow: `NewStagingWriter` per episode → `Merge` → `NewInspector().Validate`. See [docs/API.md](docs/API.md) and [examples/](examples/).

## Docs

- [CLI](docs/CLI.md) · [API](docs/API.md)
- [v2.1 layout](docs/protocol_v21.md) · [v3.0 layout](docs/protocol_v30.md)
- [Contributing](CONTRIBUTING.md) · [Changelog](CHANGELOG.md)

## Development

```bash
make test    # unit tests
make lint    # golangci-lint
make build   # ./lerobot-go
```

## License

MIT — [LICENSE](LICENSE). Copyright (c) 2026 IO-AI.TECH.
