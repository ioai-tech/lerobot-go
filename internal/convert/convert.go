package convert

import (
	"context"
	"fmt"

	"github.com/ioai-tech/lerobot-go/internal/video"
	"github.com/ioai-tech/lerobot-go/lerobot"
)

type Config struct {
	Input, Output                               string
	From, To                                    lerobot.Version
	DataFileSizeMB, VideoFileSizeMB, ChunksSize int
	Locator                                     video.Locator
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Locator == nil {
		cfg.Locator = video.NewLocator(video.Config{})
	}
	if cfg.DataFileSizeMB <= 0 {
		cfg.DataFileSizeMB = 100
	}
	if cfg.VideoFileSizeMB <= 0 {
		cfg.VideoFileSizeMB = 200
	}
	if cfg.ChunksSize <= 0 {
		cfg.ChunksSize = 1000
	}
	switch {
	case cfg.From == lerobot.V21 && cfg.To == lerobot.V30:
		return V21ToV30(ctx, cfg)
	case cfg.From == lerobot.V30 && cfg.To == lerobot.V21:
		return V30ToV21(ctx, cfg)
	default:
		return fmt.Errorf("unsupported conversion %v -> %v", cfg.From, cfg.To)
	}
}
