package video

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// SafeConcat concatenates MP4 segments into one file with exact CFR timestamps
// (frame i at t=i/fps). Stream-copy concat is intentionally avoided: ffmpeg
// `-c copy` leaves PTS discontinuities across segment boundaries that break
// LeRobot's default 1e-4 decode tolerance during training.
func SafeConcat(ctx context.Context, locator Locator, inputs []string, output string, overwrite bool, fps int) error {
	if len(inputs) == 0 {
		return fmt.Errorf("no input videos")
	}
	if len(inputs) == 1 {
		return copyFile(inputs[0], output, overwrite)
	}
	if locator == nil {
		locator = NewLocator(Config{})
	}
	ffmpeg, err := locator.FFmpegPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	if !overwrite {
		if _, err := os.Stat(output); err == nil {
			return nil
		}
	}
	if fps <= 0 {
		fps, err = probeFPS(ctx, locator, inputs[0])
		if err != nil {
			return err
		}
	}
	if fps <= 0 {
		fps = 30
	}
	return concatCFR(ctx, ffmpeg, inputs, output, fps)
}

// concatCFR re-encodes segments through the concat demuxer (one file at a time)
// and forces setpts=N/(fps*TB) so torchcodec/pyav see timestamps matching
// parquet frame_index/fps (+ episode from_timestamp).
//
// filter_complex concat is avoided: it opens every input decoder at once and
// OOMs on large v3.0 merges (hundreds of episode MP4s per 200MB shard).
func concatCFR(ctx context.Context, ffmpeg string, inputs []string, output string, fps int) error {
	listFile, err := writeConcatList(inputs)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(listFile) }()

	args := cfrConcatArgs(ffmpeg, listFile, output, fps)
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cfr concat failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func writeConcatList(inputs []string) (string, error) {
	f, err := os.CreateTemp("", "lerobot-concat-*.ffconcat")
	if err != nil {
		return "", err
	}
	for _, in := range inputs {
		abs, err := filepath.Abs(in)
		if err != nil {
			_ = f.Close()
			_ = os.Remove(f.Name())
			return "", err
		}
		escaped := strings.ReplaceAll(abs, "'", `'\''`)
		if _, err := fmt.Fprintf(f, "file '%s'\n", escaped); err != nil {
			_ = f.Close()
			_ = os.Remove(f.Name())
			return "", err
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// cfrConcatArgs builds the sequential concat-demuxer + setpts re-encode command.
func cfrConcatArgs(ffmpeg, listFile, output string, fps int) []string {
	args := []string{
		"-y", "-hide_banner", "-loglevel", "error",
		"-fflags", "+genpts",
		"-f", "concat", "-safe", "0", "-i", listFile,
	}
	args = append(args, passthroughRateArgs(ffmpeg)...)
	args = append(args,
		"-vf", fmt.Sprintf("setpts=N/(%d*TB)", fps),
		"-an", "-sn",
		"-c:v", "libx264",
		"-crf", strconv.Itoa(DefaultCRF),
		// Disable B-frames so packet order matches presentation order; LeRobot
		// readers key off presentation timestamps at i/fps.
		"-bf", "0",
		"-pix_fmt", "yuv420p",
		// fps*512 timescale makes every packet pts an exact multiple of 1/fps,
		// matching H264RemuxEncoder and LeRobot timestamp readers.
		"-video_track_timescale", strconv.Itoa(fps*512),
		"-movflags", "+faststart",
	)
	if threads := ResolveEncoderThreads(); threads > 0 {
		args = append(args, "-threads", strconv.Itoa(threads))
	}
	return append(args, output)
}

func passthroughRateArgs(ffmpeg string) []string {
	if ffmpegSupportsFPSMode(ffmpeg) {
		return []string{"-fps_mode", "passthrough"}
	}
	return []string{"-vsync", "0"}
}

var (
	ffmpegMajorOnce sync.Map // string -> int
	ffmpegVersionRE = regexp.MustCompile(`(?i)ffmpeg version (?:n)?(\d+)`)
)

func ffmpegSupportsFPSMode(ffmpeg string) bool {
	return ffmpegMajorVersion(ffmpeg) >= 5
}

func ffmpegMajorVersion(ffmpeg string) int {
	if ffmpeg == "" {
		return 0
	}
	if v, ok := ffmpegMajorOnce.Load(ffmpeg); ok {
		return v.(int)
	}
	cmd := exec.Command(ffmpeg, "-version")
	out, err := cmd.Output()
	major := 0
	if err == nil {
		if m := ffmpegVersionRE.FindSubmatch(out); len(m) == 2 {
			major, _ = strconv.Atoi(string(m[1]))
		}
	}
	ffmpegMajorOnce.Store(ffmpeg, major)
	return major
}

func probeFPS(ctx context.Context, locator Locator, path string) (int, error) {
	info, err := GetVideoInfo(ctx, locator, path)
	if err != nil {
		return 0, fmt.Errorf("probe fps for %s: %w", path, err)
	}
	v, ok := info["video.fps"].(float64)
	if !ok || v <= 0 {
		return 0, fmt.Errorf("probe fps for %s: missing video.fps", path)
	}
	return int(v), nil
}

func copyFile(src, dst string, overwrite bool) error {
	if !overwrite {
		if _, err := os.Stat(dst); err == nil {
			return nil
		}
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
