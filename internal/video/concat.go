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
// (frame i at t=i/fps). When every segment is already CFR at fps (the converter
// episode path), concat demuxer + bitstream copy keeps PTS continuous and
// avoids a second libx264 pass. Re-encode is the fallback if copy fails
// ValidateMP4 (mismatched timebases, B-frames, or PTS gaps).
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
	counts := make([]int, len(inputs))
	frames := 0
	counted := true
	for i, in := range inputs {
		n, err := probeFrameCount(ctx, locator, in)
		if err != nil {
			counted = false
			break
		}
		counts[i] = n
		frames += n
	}
	if counted && frames > 0 {
		if err := concatCopy(ctx, ffmpeg, inputs, counts, output, fps); err == nil {
			if err := ValidateMP4(ctx, locator, output, frames, fps); err == nil {
				return nil
			}
		}
		_ = os.Remove(output)
	}
	return concatCFR(ctx, ffmpeg, inputs, output, fps)
}

func concatCopy(ctx context.Context, ffmpeg string, inputs []string, counts []int, output string, fps int) error {
	listFile, err := writeConcatList(inputs, fps, counts)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(listFile) }()
	args := []string{
		"-y", "-hide_banner", "-loglevel", "error",
		"-f", "concat", "-safe", "0", "-i", listFile,
		"-an", "-sn",
		"-c:v", "copy",
		"-video_track_timescale", strconv.Itoa(fps * 512),
		"-movflags", "+faststart",
		output,
	}
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("copy concat failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// concatCFR re-encodes segments through the concat demuxer (one file at a time)
// and forces setpts=N/(fps*TB) so torchcodec/pyav see timestamps matching
// parquet frame_index/fps (+ episode from_timestamp).
//
// filter_complex concat is avoided: it opens every input decoder at once and
// OOMs on large v3.0 merges (hundreds of episode MP4s per 200MB shard).
func concatCFR(ctx context.Context, ffmpeg string, inputs []string, output string, fps int) error {
	listFile, err := writeConcatList(inputs, 0, nil)
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

func writeConcatList(inputs []string, fps int, counts []int) (string, error) {
	f, err := os.CreateTemp("", "lerobot-concat-*.ffconcat")
	if err != nil {
		return "", err
	}
	withDur := fps > 0 && len(counts) == len(inputs)
	for i, in := range inputs {
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
		if withDur && counts[i] > 0 {
			// Pin each segment's timeline to n/fps so bitstream-copy concat
			// starts the next file on an exact i/fps boundary.
			if _, err := fmt.Fprintf(f, "duration %g\n", float64(counts[i])/float64(fps)); err != nil {
				_ = f.Close()
				_ = os.Remove(f.Name())
				return "", err
			}
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

func probeFrameCount(ctx context.Context, locator Locator, path string) (int, error) {
	ffprobe, err := locator.FFprobePath()
	if err != nil {
		return 0, err
	}
	out, err := exec.CommandContext(ctx, ffprobe,
		"-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=nb_frames",
		"-of", "default=noprint_wrappers=1:nokey=0", path,
	).Output()
	if err != nil {
		return 0, err
	}
	fields := parseProbeFields(string(out))
	if n, err := strconv.Atoi(fields["nb_frames"]); err == nil && n > 0 {
		return n, nil
	}
	out, err = exec.CommandContext(ctx, ffprobe,
		"-v", "error", "-select_streams", "v:0",
		"-count_packets", "-show_entries", "stream=nb_read_packets",
		"-of", "default=noprint_wrappers=1:nokey=0", path,
	).Output()
	if err != nil {
		return 0, err
	}
	fields = parseProbeFields(string(out))
	n, err := strconv.Atoi(fields["nb_read_packets"])
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("probe frame count for %s: %v", path, fields["nb_read_packets"])
	}
	return n, nil
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
