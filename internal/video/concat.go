package video

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

// concatCFR re-encodes segments through the concat filter and forces
// setpts=N/(fps*TB) so torchcodec/pyav see timestamps matching parquet
// frame_index/fps (+ episode from_timestamp).
func concatCFR(ctx context.Context, ffmpeg string, inputs []string, output string, fps int) error {
	args := []string{"-y", "-hide_banner", "-loglevel", "error"}
	for _, in := range inputs {
		args = append(args, "-i", in)
	}
	var filters []string
	var maps []string
	for i := range inputs {
		filters = append(filters, fmt.Sprintf("[%d:v]setpts=PTS-STARTPTS[v%d]", i, i))
		maps = append(maps, fmt.Sprintf("[v%d]", i))
	}
	filter := strings.Join(filters, ";") + ";" +
		strings.Join(maps, "") +
		fmt.Sprintf("concat=n=%d:v=1:a=0,setpts=N/(%d*TB)[outv]", len(inputs), fps)
	args = append(args,
		"-filter_complex", filter,
		"-map", "[outv]",
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
	args = append(args, output)
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cfr concat failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
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
