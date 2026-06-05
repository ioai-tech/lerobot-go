// Package lerobot implements the HuggingFace LeRobot dataset writer and inspector
// for on-disk formats v2.1 and v3.0.
//
// # Supported versions
//
//   - V21 (codebase v2.1): per-episode parquet, optional mp4 videos or embedded images
//   - V30 (codebase v3.0): chunked parquet, meta/tasks.parquet, chunked videos
//
// # Typical workflows
//
// Parallel ingest (recommended for robotics pipelines):
//
//   - NewStagingWriter per episode under a shared staging root (ep_NNNNNN dirs)
//   - Merge to produce the final dataset (CLI: lerobot-go create)
//
// Serial ingest:
//
//   - Create opens Root/_staging, AddFrame / SaveEpisode per episode
//   - Finalize runs Merge into Root
//
// Validation:
//
//   - NewInspector().Validate or ValidateStrict on a dataset directory
//
// CLI commands validate, convert, create, and merge call the same internal logic.
// See docs/API.md and examples/ for runnable code.
package lerobot
