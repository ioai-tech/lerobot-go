package video

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// ExtractSegment copies a time range from src into dst using ffmpeg.
func ExtractSegment(ctx context.Context, locator Locator, src, dst string, fromSec, toSec float64, overwrite bool) error {
	ffmpeg, err := locator.FFmpegPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if !overwrite {
		if _, err := os.Stat(dst); err == nil {
			return nil
		}
	}
	args := []string{
		"-y",
		"-ss", strconv.FormatFloat(fromSec, 'f', 6, 64),
		"-to", strconv.FormatFloat(toSec, 'f', 6, 64),
		"-i", src,
		"-c", "copy",
		"-movflags", "+faststart",
		dst,
	}
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg extract: %w (%s)", err, string(out))
	}
	return nil
}
