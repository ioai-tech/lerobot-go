package video

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSafeConcatProducesExactFPSTimestamps(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}

	const fps = 30
	dir := t.TempDir()
	var segments []string
	for i := 0; i < 4; i++ {
		seg := filepath.Join(dir, "ep"+strconv.Itoa(i)+".mp4")
		gen := exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
			"-f", "lavfi", "-i", "color=c=black:s=64x64:r="+strconv.Itoa(fps),
			"-frames:v", "25",
			"-an", "-sn",
			"-c:v", "libx264", "-preset", "ultrafast", "-crf", "30",
			"-pix_fmt", "yuv420p",
			seg,
		)
		if out, err := gen.CombinedOutput(); err != nil {
			t.Fatalf("generate segment %d: %v\n%s", i, err, out)
		}
		segments = append(segments, seg)
	}

	out := filepath.Join(dir, "cat.mp4")
	if err := SafeConcat(context.Background(), nil, segments, out, true, fps); err != nil {
		t.Fatalf("SafeConcat: %v", err)
	}
	if err := ValidateMP4(context.Background(), nil, out, 100, fps); err != nil {
		t.Fatalf("ValidateMP4: %v", err)
	}

	probe := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "frame=pts_time,best_effort_timestamp_time,pkt_pts_time",
		"-of", "csv=p=0", out)
	b, err := probe.Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	var timestamps []float64
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		ts, ok := firstProbeTimestamp(line)
		if !ok {
			t.Fatalf("parse pts %q", line)
		}
		timestamps = append(timestamps, ts)
	}
	if len(timestamps) != 100 {
		t.Fatalf("want 100 frames got %d", len(timestamps))
	}
	const tol = 1e-4
	var maxDiff float64
	for i, ts := range timestamps {
		diff := math.Abs(ts - float64(i)/float64(fps))
		if diff > maxDiff {
			maxDiff = diff
		}
		if diff > tol {
			t.Fatalf("pts drift at frame %d: pts=%v want=%v diff=%v", i, ts, float64(i)/float64(fps), diff)
		}
	}
	t.Logf("max pts drift=%g", maxDiff)

	// Regression: stream-copy concat drifts past LeRobot tolerance after the
	// first segment boundary. Keep this assertion so we never silently revert.
	if maxDiff >= tol {
		t.Fatalf("unexpected drift %g", maxDiff)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal(err)
	}
}

func TestSafeConcatSingleFileCopies(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mp4")
	gen := exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=black:s=64x64:r=30",
		"-frames:v", "5", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generate: %v\n%s", err, out)
	}
	dst := filepath.Join(dir, "dst.mp4")
	if err := SafeConcat(context.Background(), nil, []string{src}, dst, true, 30); err != nil {
		t.Fatalf("SafeConcat: %v", err)
	}
	srcInfo, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	dstInfo, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if srcInfo.Size() != dstInfo.Size() {
		t.Fatalf("single-file concat should copy bytes: src=%d dst=%d", srcInfo.Size(), dstInfo.Size())
	}
}
