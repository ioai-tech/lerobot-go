package mcap2lerobot

import (
	"context"
	"fmt"

	"github.com/ioai-tech/lerobot-go/lerobot"
)

// Config holds future mcap conversion options.
type Config struct {
	InputPath   string
	OutputRoot  string
	StagingRoot string
	FPS         int
	Version     lerobot.Version
	MaxWorkers  int
}

// Convert is a Phase 4 placeholder that validates config and documents the integration point.
func Convert(ctx context.Context, cfg Config) error {
	if cfg.InputPath == "" {
		return fmt.Errorf("input path required")
	}
	if cfg.OutputRoot == "" {
		return fmt.Errorf("output root required")
	}
	_ = ctx
	return fmt.Errorf("mcap2lerobot Go converter not yet implemented; use lerobot.NewStagingWriter + lerobot.Merge from ported pipeline")
}
