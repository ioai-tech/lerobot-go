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
		if err := validateExactFPSTimestamps(ctx, ffprobe, path, expectedFrames, fps); err != nil {
			return err
		}
	}
	return nil
}

// validateExactFPSTimestamps ensures packet PTS match i/fps within LeRobot's
// default training tolerance (1e-4s). Drift here surfaces as torchcodec
// AssertionError when decoding dataset videos.
func validateExactFPSTimestamps(ctx context.Context, ffprobe, path string, expectedFrames, fps int) error {
	const toleranceS = 1e-4
	out, err := exec.CommandContext(ctx, ffprobe,
		"-v", "error", "-select_streams", "v:0",
		"-show_entries", "frame=pts_time,best_effort_timestamp_time,pkt_pts_time",
		"-of", "csv=p=0", path,
	).Output()
	if err != nil {
		return fmt.Errorf("ffprobe pts validation failed for %s: %w", path, err)
	}
	var timestamps []float64
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		ts, ok := firstProbeTimestamp(line)
		if !ok {
			return fmt.Errorf("ffprobe pts validation failed for %s: parse %q", path, line)
		}
		timestamps = append(timestamps, ts)
	}
	if len(timestamps) == 0 {
		return fmt.Errorf("ffprobe pts validation failed for %s: no frames", path)
	}
	if expectedFrames > 0 && len(timestamps) != expectedFrames {
		return fmt.Errorf("video frame count mismatch for %s: timed=%d expected=%d", path, len(timestamps), expectedFrames)
	}
	for i, ts := range timestamps {
		want := float64(i) / float64(fps)
		if math.Abs(ts-want) > toleranceS {
			return fmt.Errorf("video timestamp drift for %s at frame %d: pts=%.6f want=%.6f (tol=%g)", path, i, ts, want, toleranceS)
		}
	}
	return nil
}

func firstProbeTimestamp(line string) (float64, bool) {
	for _, part := range strings.Split(line, ",") {
		part = strings.TrimSpace(part)
		if part == "" || part == "N/A" {
			continue
		}
		ts, err := strconv.ParseFloat(part, 64)
		if err == nil {
			return ts, true
		}
	}
	return 0, false
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
