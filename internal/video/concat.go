package video

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SafeConcat concatenates MP4 files with DTS monotonicity (tier-1 ffmpeg concat).
func SafeConcat(ctx context.Context, locator Locator, inputs []string, output string, overwrite bool) error {
	if len(inputs) == 0 {
		return fmt.Errorf("no input videos")
	}
	if len(inputs) == 1 {
		return copyFile(inputs[0], output, overwrite)
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

	listFile, err := os.CreateTemp("", "concat-*.txt")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(listFile.Name()) }()
	for _, in := range inputs {
		abs, err := filepath.Abs(in)
		if err != nil {
			return err
		}
		escaped := strings.ReplaceAll(abs, "'", "'\\''")
		if _, err := fmt.Fprintf(listFile, "file '%s'\n", escaped); err != nil {
			return err
		}
	}
	if err := listFile.Close(); err != nil {
		return err
	}

	// Tier 1: concat demuxer with genpts
	args := []string{
		"-y",
		"-fflags", "+genpts",
		"-f", "concat",
		"-safe", "0",
		"-i", listFile.Name(),
		"-c", "copy",
		"-movflags", "+faststart",
		output,
	}
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Tier 2: re-mux with setpts filter per input
		if err2 := concatWithSetpts(ctx, ffmpeg, inputs, output); err2 != nil {
			return fmt.Errorf("concat failed: %v (%s); setpts fallback: %w", err, string(out), err2)
		}
	}
	return nil
}

func concatWithSetpts(ctx context.Context, ffmpeg string, inputs []string, output string) error {
	args := []string{"-y"}
	for _, in := range inputs {
		args = append(args, "-i", in)
	}
	var filters []string
	var maps []string
	for i := range inputs {
		filters = append(filters, fmt.Sprintf("[%d:v]setpts=PTS-STARTPTS[v%d]", i, i))
		maps = append(maps, fmt.Sprintf("[v%d]", i))
	}
	filter := strings.Join(filters, ";") + ";" + strings.Join(maps, "") + fmt.Sprintf("concat=n=%d:v=1:a=0[outv]", len(inputs))
	args = append(args, "-filter_complex", filter, "-map", "[outv]", "-c:v", "libx264", "-crf", strconvItoa(DefaultCRF), "-movflags", "+faststart", output)
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	return cmd.Run()
}

func strconvItoa(n int) string {
	return fmt.Sprintf("%d", n)
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
