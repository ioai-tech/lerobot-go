package video

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
)

func ValidateMP4(ctx context.Context, locator Locator, path string, expectedFrames, fps int) error {
	if expectedFrames <= 0 {
		return nil
	}
	if locator == nil {
		locator = NewLocator(Config{})
	}
	ffmpeg, err := locator.FFmpegPath()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, ffmpeg, "-v", "warning", "-i", path, "-f", "null", "-")
	var stderr bytes.Buffer
	cmd.Stdout = &stderr
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("decode validation failed for %s: %w: %s", path, err, strings.TrimSpace(stderr.String()))
	}
	if warnings := filteredDecodeWarnings(stderr.String()); warnings != "" {
		return fmt.Errorf("decode validation warnings for %s: %s", path, warnings)
	}

	ffprobe, err := locator.FFprobePath()
	if err != nil {
		return err
	}
	out, err := exec.CommandContext(ctx, ffprobe,
		"-v", "error", "-count_frames", "-select_streams", "v:0",
		"-show_entries", "stream=nb_read_frames,r_frame_rate,duration",
		"-of", "default=noprint_wrappers=1:nokey=0", path,
	).Output()
	if err != nil {
		return fmt.Errorf("ffprobe validation failed for %s: %w", path, err)
	}
	fields := parseProbeFields(string(out))
	got, err := strconv.Atoi(fields["nb_read_frames"])
	if err != nil {
		return fmt.Errorf("ffprobe missing nb_read_frames for %s: %w", path, err)
	}
	if got != expectedFrames {
		return fmt.Errorf("video frame count mismatch for %s: decoded=%d expected=%d", path, got, expectedFrames)
	}
	if fps > 0 {
		if gotFPS, err := parseFrameRate(fields["r_frame_rate"]); err == nil && gotFPS > 0 && math.Abs(gotFPS-float64(fps)) > 0.01 {
			return fmt.Errorf("video fps mismatch for %s: got=%.3f expected=%d", path, gotFPS, fps)
		}
		if dur, err := strconv.ParseFloat(fields["duration"], 64); err == nil && dur > 0 {
			want := float64(expectedFrames) / float64(fps)
			if math.Abs(dur-want) > math.Max(0.2, 5.0/float64(fps)) {
				return fmt.Errorf("video duration mismatch for %s: got=%.3f expected=%.3f", path, dur, want)
			}
		}
	}
	return nil
}

func filteredDecodeWarnings(out string) string {
	var kept []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "deprecated pixel format used") {
			continue
		}
		if strings.Contains(line, "Application provided invalid, non monotonically increasing dts to muxer") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func parseProbeFields(out string) map[string]string {
	fields := make(map[string]string)
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			fields[k] = v
		}
	}
	return fields
}
