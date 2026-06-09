package video

import (
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestH264RemuxEncoderRequiresFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "out.mp4")
	enc, err := NewH264RemuxEncoder(context.Background(), nil, 30, out)
	if err != nil {
		t.Fatalf("NewH264RemuxEncoder: %v", err)
	}
	_ = enc.Close()
}

func TestH264RemuxFFmpegArgsOmitsGenpts(t *testing.T) {
	args := h264RemuxFFmpegArgs(30, "/tmp/out.mp4")
	if slices.Contains(args, "+genpts") || strings.Contains(strings.Join(args, " "), "genpts") {
		t.Fatalf("remux args should not use genpts: %v", args)
	}
}
