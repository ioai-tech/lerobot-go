# lerobot-go

[English](README.md) · 由 [IO-AI.TECH](https://io-ai.tech) 维护

[![CI](https://github.com/ioai-tech/lerobot-go/actions/workflows/ci.yml/badge.svg)](https://github.com/ioai-tech/lerobot-go/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/ioai-tech/lerobot-go)](https://goreportcard.com/report/github.com/ioai-tech/lerobot-go)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/ioai-tech/lerobot-go)](https://github.com/ioai-tech/lerobot-go/releases)

[LeRobot](https://github.com/huggingface/lerobot) 数据集（v2.1 / v3.0）的 Go 库与 CLI：写入、校验、转换、合并。目录结构与 HuggingFace 官方格式一致。

**环境要求：** Go 1.26+；处理视频特征时需 `ffmpeg` / `ffprobe` 在 `PATH` 中。

## 安装

```bash
go install github.com/ioai-tech/lerobot-go/cmd/lerobot-go@latest
```

预编译包见 [Releases](https://github.com/ioai-tech/lerobot-go/releases)（`linux` / `darwin` / `windows` × `amd64` / `arm64`，含 `checksums.txt`）。

## CLI

```bash
lerobot-go validate ./dataset
lerobot-go convert -i ./v21 -o ./v30 --to v3.0
lerobot-go convert -i ./v30 -o ./v21 --to v2.1
lerobot-go create -o ./dataset --version v3.0 --fps 30 --features ./features.json
lerobot-go merge -o ./merged --to v3.0 -i ./a -i ./b
```

| 命令 | 说明 |
|------|------|
| `validate` | 校验 v2.1 / v3.0 目录结构 |
| `convert` | v2.1 与 v3.0 互转 |
| `create` | 将 `_staging/ep_*` 落盘为正式数据集 |
| `merge` | 合并多个数据集 |
| `version` / `completion` | 版本信息与 Shell 补全 |

全局参数：`-v`、`--ffmpeg-path`、`--json`。完整说明见 [docs/CLI_zh.md](docs/CLI_zh.md)。

## 库 API

```go
import "github.com/ioai-tech/lerobot-go/lerobot"
```

典型流程：每集 `NewStagingWriter` → `Merge` → `NewInspector().Validate`。详见 [docs/API_zh.md](docs/API_zh.md) 与 [examples/](examples/)。

## 文档

- [CLI](docs/CLI_zh.md) · [API](docs/API_zh.md)
- [v2.1 布局](docs/protocol_v21.md) · [v3.0 布局](docs/protocol_v30.md)
- [Contributing](CONTRIBUTING.md) · [Changelog](CHANGELOG.md)

## 开发

```bash
make test    # 单元测试
make lint    # golangci-lint
make build   # 编译 ./lerobot-go
```

## 许可证

MIT — [LICENSE](LICENSE)。Copyright (c) 2026 IO-AI.TECH。
