package stats

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/ioai-tech/lerobot-go/internal/video"
)

func sampleVideo(ctx context.Context, locator video.Locator, path string, length int, full bool) ([][][][]uint8, int, error) {
	if path == "" {
		return nil, 0, fmt.Errorf("empty video path")
	}
	if length <= 0 {
		return nil, 0, fmt.Errorf("episode length must be > 0 to sample %s", path)
	}
	if locator == nil {
		locator = video.NewLocator(video.Config{})
	}
	ffmpeg, err := locator.FFmpegPath()
	if err != nil {
		return nil, 0, err
	}
	width, height, err := probeVideoSize(ctx, locator, path)
	if err != nil {
		return nil, 0, err
	}
	indices := sampleIndices(length, full)
	if len(indices) == 0 {
		return nil, 0, nil
	}
	wanted := make(map[int]struct{}, len(indices))
	for _, idx := range indices {
		wanted[idx] = struct{}{}
	}
	lastWanted := indices[len(indices)-1]

	cmd := exec.CommandContext(ctx, ffmpeg,
		"-hide_banner", "-loglevel", "error",
		"-i", path,
		"-f", "rawvideo", "-pix_fmt", "rgb24",
		"pipe:1",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, 0, err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, 0, fmt.Errorf("start ffmpeg: %w", err)
	}

	frameSize := width * height * 3
	buf := make([]byte, frameSize)
	kept := make(map[int][][][]uint8, len(wanted))
	frameIdx := 0
	for {
		if _, err := io.ReadFull(stdout, buf); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			_ = cmd.Wait()
			return nil, 0, fmt.Errorf("read rgb frame %d from %s: %w: %s", frameIdx, path, err, strings.TrimSpace(stderr.String()))
		}
		if _, ok := wanted[frameIdx]; ok {
			chw := rgb24ToCHW(buf, width, height)
			kept[frameIdx] = autoDownsampleHeightWidth(chw, 150, 300)
		}
		frameIdx++
		if frameIdx > lastWanted && len(kept) == len(wanted) {
			break
		}
	}
	_ = stdout.Close()
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	_ = cmd.Wait()
	if len(kept) == 0 {
		return nil, 0, fmt.Errorf("no sampled frames from %s (decoded %d, want %d indices): %s",
			path, frameIdx, len(indices), strings.TrimSpace(stderr.String()))
	}

	out := make([][][][]uint8, 0, len(indices))
	for _, idx := range indices {
		img, ok := kept[idx]
		if !ok {
			continue
		}
		out = append(out, img)
	}
	return out, len(out), nil
}

func probeVideoSize(ctx context.Context, locator video.Locator, path string) (width, height int, err error) {
	info, err := video.GetVideoInfo(ctx, locator, path)
	if err != nil {
		return 0, 0, err
	}
	w, _ := info["video.width"].(int)
	h, _ := info["video.height"].(int)
	if w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("probe %s: missing video width/height", path)
	}
	return w, h, nil
}

func rgb24ToCHW(rgb []byte, width, height int) [][][]uint8 {
	chw := make([][][]uint8, 3)
	for c := 0; c < 3; c++ {
		plane := make([][]uint8, height)
		for y := 0; y < height; y++ {
			row := make([]uint8, width)
			for x := 0; x < width; x++ {
				row[x] = rgb[(y*width+x)*3+c]
			}
			plane[y] = row
		}
		chw[c] = plane
	}
	return chw
}
