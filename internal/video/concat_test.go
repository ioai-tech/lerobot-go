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

func TestCFRConcatArgsUsesConcatDemuxer(t *testing.T) {
	args := cfrConcatArgs("ffmpeg", "/tmp/list.ffconcat", "/tmp/out.mp4", 30)
	if countFlag(args, "-i") != 1 {
		t.Fatalf("want a single -i (concat list), got %v", args)
	}
	if !containsPair(args, "-f", "concat") {
		t.Fatalf("missing -f concat: %v", args)
	}
	if contains(args, "-filter_complex") {
		t.Fatalf("must not use filter_complex concat: %v", args)
	}
	if !containsPair(args, "-vf", "setpts=N/(30*TB)") {
		t.Fatalf("missing setpts: %v", args)
	}
	if !containsPair(args, "-video_track_timescale", "15360") {
		t.Fatalf("missing timescale: %v", args)
	}
	if args[len(args)-1] != "/tmp/out.mp4" {
		t.Fatalf("output should be last: %v", args)
	}
}

func TestWriteConcatListEscapesQuotes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ep's.mp4")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	list, err := writeConcatList([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(list) }()
	data, err := os.ReadFile(list)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, `'\''`) {
		t.Fatalf("expected escaped quote in %q", got)
	}
	if !strings.HasPrefix(got, "file '") || !strings.HasSuffix(got, "'\n") {
		t.Fatalf("unexpected list format %q", got)
	}
}

func TestSafeConcatManySegmentsExactFPS(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}

	const (
		fps      = 30
		nSeg     = 32
		frames   = 3
		expected = nSeg * frames
	)
	dir := t.TempDir()
	segments := make([]string, nSeg)
	for i := 0; i < nSeg; i++ {
		seg := filepath.Join(dir, "ep"+strconv.Itoa(i)+".mp4")
		gen := exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
			"-f", "lavfi", "-i", "color=c=black:s=64x64:r="+strconv.Itoa(fps),
			"-frames:v", strconv.Itoa(frames),
			"-an", "-sn",
			"-c:v", "libx264", "-preset", "ultrafast", "-crf", "30",
			"-pix_fmt", "yuv420p",
			seg,
		)
		if out, err := gen.CombinedOutput(); err != nil {
			t.Fatalf("generate segment %d: %v\n%s", i, err, out)
		}
		segments[i] = seg
	}

	out := filepath.Join(dir, "cat.mp4")
	if err := SafeConcat(context.Background(), nil, segments, out, true, fps); err != nil {
		t.Fatalf("SafeConcat: %v", err)
	}
	if err := ValidateMP4(context.Background(), nil, out, expected, fps); err != nil {
		t.Fatalf("ValidateMP4: %v", err)
	}
	assertExactFPSTimestamps(t, out, expected, fps)
}

func assertExactFPSTimestamps(t *testing.T, path string, wantFrames, fps int) {
	t.Helper()
	probe := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "frame=pts_time,best_effort_timestamp_time,pkt_pts_time",
		"-of", "csv=p=0", path)
	b, err := probe.Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	const tol = 1e-4
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
	if len(timestamps) != wantFrames {
		t.Fatalf("want %d frames got %d", wantFrames, len(timestamps))
	}
	for i, ts := range timestamps {
		diff := math.Abs(ts - float64(i)/float64(fps))
		if diff > tol {
			t.Fatalf("pts drift at frame %d: pts=%v want=%v diff=%v", i, ts, float64(i)/float64(fps), diff)
		}
	}
}

func countFlag(args []string, flag string) int {
	n := 0
	for _, a := range args {
		if a == flag {
			n++
		}
	}
	return n
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func containsPair(args []string, flag, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}
