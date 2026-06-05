package video

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

const DefaultCRF = 25

type EncodeConfig struct {
	Locator    Locator
	VCodec     string
	CRF        int
	FPS        int
	Threads    int
	PNGPattern string
	OutputPath string
}

func EncodeFromPNGDir(ctx context.Context, cfg EncodeConfig) error {
	ffmpeg, err := cfg.Locator.FFmpegPath()
	if err != nil {
		return err
	}
	if cfg.CRF <= 0 {
		cfg.CRF = DefaultCRF
	}
	if cfg.VCodec == "" {
		cfg.VCodec = "libx264"
	}
	args := []string{
		"-y",
		"-framerate", strconv.Itoa(cfg.FPS),
		"-i", cfg.PNGPattern,
		"-c:v", cfg.VCodec,
		"-crf", strconv.Itoa(cfg.CRF),
		"-pix_fmt", "yuv420p",
		"-movflags", "+faststart",
	}
	if cfg.Threads > 0 {
		args = append(args, "-threads", strconv.Itoa(cfg.Threads))
	}
	args = append(args, cfg.OutputPath)
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func DurationSeconds(ctx context.Context, locator Locator, path string) (float64, error) {
	ffprobe, err := locator.FFprobePath()
	if err != nil {
		return 0, err
	}
	cmd := exec.CommandContext(ctx, ffprobe,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseFloat(string(trimSpace(out)), 64)
	if err != nil {
		return 0, fmt.Errorf("parse duration: %w", err)
	}
	return v, nil
}

func FileSizeMB(path string) (float64, error) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return float64(st.Size()) / (1024 * 1024), nil
}

func ImageDir(videoKey string, episode int, root string) string {
	return filepath.Join(root, "images", videoKey, fmt.Sprintf("episode-%06d", episode))
}

func trimSpace(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\n' || b[0] == '\r' || b[0] == '\t') {
		b = b[1:]
	}
	for len(b) > 0 && (b[len(b)-1] == ' ' || b[len(b)-1] == '\n' || b[len(b)-1] == '\r' || b[len(b)-1] == '\t') {
		b = b[:len(b)-1]
	}
	return b
}
