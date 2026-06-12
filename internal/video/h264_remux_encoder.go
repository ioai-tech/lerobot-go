package video

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// H264RemuxEncoder muxes Annex-B access units into MP4 via ffmpeg copy (no RGB round-trip).
type H264RemuxEncoder struct {
	locator Locator
	fps     int
	output  string

	cmd   *exec.Cmd
	stdin io.WriteCloser
}

// NewH264RemuxEncoder starts ffmpeg in h264 copy mode.
func NewH264RemuxEncoder(ctx context.Context, locator Locator, fps int, outputPath string) (*H264RemuxEncoder, error) {
	if locator == nil {
		locator = NewLocator(Config{})
	}
	ffmpeg, err := locator.FFmpegPath()
	if err != nil {
		return nil, err
	}
	if fps <= 0 {
		fps = 30
	}
	if err := os.MkdirAll(dirOf(outputPath), 0o755); err != nil {
		return nil, err
	}
	args := h264RemuxFFmpegArgs(fps, outputPath)
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, err
	}
	return &H264RemuxEncoder{
		locator: locator,
		fps:     fps,
		output:  outputPath,
		cmd:     cmd,
		stdin:   stdin,
	}, nil
}

// WriteAccessUnits writes ordered access units (nil entries are skipped).
func (e *H264RemuxEncoder) WriteAccessUnits(aus [][]byte) error {
	if e.stdin == nil {
		return fmt.Errorf("encoder closed or not started")
	}
	for _, au := range aus {
		if len(au) == 0 {
			continue
		}
		if _, err := e.stdin.Write(au); err != nil {
			return err
		}
	}
	return nil
}

// Close flushes stdin and waits for ffmpeg.
func (e *H264RemuxEncoder) Close() error {
	var firstErr error
	if e.stdin != nil {
		if err := e.stdin.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		e.stdin = nil
	}
	if e.cmd != nil {
		if err := e.cmd.Wait(); err != nil && firstErr == nil {
			firstErr = err
		}
		e.cmd = nil
	}
	return firstErr
}

// OutputPath returns the destination mp4 path.
func (e *H264RemuxEncoder) OutputPath() string {
	return e.output
}

// FrameCount probes packet count via ffprobe when available.
func (e *H264RemuxEncoder) FrameCount(ctx context.Context) (int, error) {
	ffprobe, err := e.locator.FFprobePath()
	if err != nil {
		return 0, err
	}
	cmd := exec.CommandContext(ctx, ffprobe,
		"-v", "error", "-select_streams", "v:0",
		"-count_packets", "-show_entries", "stream=nb_read_packets",
		"-of", "csv=p=0", e.output,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, err
	}
	return n, nil
}

func h264RemuxFFmpegArgs(fps int, outputPath string) []string {
	return []string{
		"-y", "-hide_banner", "-loglevel", "error",
		"-f", "h264",
		// -framerate alone can be overridden by SPS VUI timing (streams then
		// mux at 25fps and drift against parquet timestamps); input -r forces
		// constant frame rate at the demuxer.
		"-framerate", strconv.Itoa(fps),
		"-r", strconv.Itoa(fps),
		"-i", "pipe:0",
		"-c:v", "copy",
		// fps*512 timescale makes every packet pts an exact multiple of
		// 1/fps, which timestamp-based dataset readers rely on.
		"-video_track_timescale", strconv.Itoa(fps * 512),
		"-movflags", "+faststart",
		outputPath,
	}
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}
