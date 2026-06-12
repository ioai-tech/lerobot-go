package video

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
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

// Regression: -framerate alone is overridden by SPS VUI timing on some camera
// streams, muxing at 25fps so the video drifts against parquet timestamps.
// The remuxed mp4 must carry exact 1/fps pts steps starting at zero.
func TestH264RemuxEncoderProducesExactFPSTimestamps(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src.h264")
	gen := exec.Command(ffmpeg, "-y", "-v", "error",
		"-f", "lavfi", "-i", "testsrc=size=64x64:rate=30", "-frames:v", "10",
		"-c:v", "libx264", "-g", "30", "-bf", "0", "-pix_fmt", "yuv420p",
		"-f", "h264", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generate source: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "out.mp4")
	enc, err := NewH264RemuxEncoder(context.Background(), nil, 30, out)
	if err != nil {
		t.Fatalf("NewH264RemuxEncoder: %v", err)
	}
	if err := enc.WriteAccessUnits([][]byte{raw}); err != nil {
		t.Fatalf("WriteAccessUnits: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	probe := exec.Command(ffprobe, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "packet=pts", "-of", "csv=p=0", out)
	b, err := probe.Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	var pts []int64
	for _, line := range strings.Fields(strings.TrimSpace(string(b))) {
		v, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			t.Fatalf("parse pts %q: %v", line, err)
		}
		pts = append(pts, v)
	}
	if len(pts) != 10 {
		t.Fatalf("want 10 packets got %d", len(pts))
	}
	if pts[0] != 0 {
		t.Fatalf("first pts must be 0, got %d", pts[0])
	}
	const step = 512 // timescale fps*512 -> exactly 1/fps per frame
	for i := 1; i < len(pts); i++ {
		if pts[i]-pts[i-1] != step {
			t.Fatalf("pts delta at %d is %d, want %d (drifts against parquet timestamps)", i, pts[i]-pts[i-1], step)
		}
	}
}
