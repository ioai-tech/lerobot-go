# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.2.3] - 2026-08-17

### Fixed

- v3.0 `SafeConcat` now uses the concat demuxer plus a single `setpts` re-encode, so large merges no longer open every episode MP4 in one `filter_complex` graph (OOM / `cfr concat failed: signal: killed`)
- Default worker count follows `GOMAXPROCS` (container CPU quota) instead of `NumCPU()-2`

## [Unreleased]

### Changed

- Rebranded to [IO-AI.TECH](https://io-ai.tech); official repository is [ioai-tech/lerobot-go](https://github.com/ioai-tech/lerobot-go)
- Module path: `github.com/ioai-tech/lerobot-go`; CLI binary renamed to `lerobot-go`

### Added

- GitHub Actions release workflow (GoReleaser on `v*` tags, multi-platform CLI archives)
- Bilingual README (English + 简体中文) with CLI and library quick starts
- `docs/CLI.md`, `docs/API.md`, and Chinese mirrors
- `examples/write_v30` and `examples/validate_dataset`
- CI workflow (test + golangci-lint), Makefile, GoReleaser config
- Public `StatsMode` and `FFmpegConfig` types for third-party API use
- MIT LICENSE

### Changed

- `create` CLI command documented as equivalent to API `Merge` (staging finalize)

## [0.2.0] - 2026-06-05

### Added

- LeRobot v2.1 and v3.0 writer with parallel staging and merge
- CLI: `validate`, `convert`, `create`, `merge`, `version`, `completion`
- v2.1-img support (image dtype embedded in parquet)
- Episode stats modes: `sampled` and `full`
- Video `features[].info` via ffprobe

[Unreleased]: https://github.com/ioai-tech/lerobot-go/compare/v1.2.3...HEAD
[1.2.3]: https://github.com/ioai-tech/lerobot-go/releases/tag/v1.2.3
[0.2.0]: https://github.com/ioai-tech/lerobot-go/releases/tag/v0.2.0
