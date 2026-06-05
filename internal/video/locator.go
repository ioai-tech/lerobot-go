package video

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

type Config struct {
	FFmpegPath  string
	FFprobePath string
}

type Locator interface {
	FFmpegPath() (string, error)
	FFprobePath() (string, error)
}

type chainLocator struct {
	candidates []Locator
}

func (c *chainLocator) FFmpegPath() (string, error) {
	for _, l := range c.candidates {
		if p, err := l.FFmpegPath(); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("ffmpeg not found")
}

func (c *chainLocator) FFprobePath() (string, error) {
	for _, l := range c.candidates {
		if p, err := l.FFprobePath(); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("ffprobe not found")
}

func NewLocator(cfg Config) Locator {
	var chain []Locator
	if cfg.FFmpegPath != "" {
		chain = append(chain, explicitLocator{ffmpeg: cfg.FFmpegPath, ffprobe: cfg.FFprobePath})
	}
	chain = append(chain, bundledLocator{})
	chain = append(chain, pathLocator{})
	return &chainLocator{candidates: chain}
}

type explicitLocator struct {
	ffmpeg, ffprobe string
}

func (e explicitLocator) FFmpegPath() (string, error) {
	if e.ffmpeg == "" {
		return "", fmt.Errorf("empty")
	}
	return e.ffmpeg, nil
}

func (e explicitLocator) FFprobePath() (string, error) {
	if e.ffprobe != "" {
		return e.ffprobe, nil
	}
	return "", fmt.Errorf("empty")
}

type pathLocator struct{}

func (pathLocator) FFmpegPath() (string, error)  { return exec.LookPath("ffmpeg") }
func (pathLocator) FFprobePath() (string, error) { return exec.LookPath("ffprobe") }

type bundledLocator struct{}

var (
	extractOnce sync.Once
	extractErr  error
	ffmpegBin   string
	ffprobeBin  string
)

func (bundledLocator) FFmpegPath() (string, error) {
	extractOnce.Do(extractBundled)
	if extractErr != nil {
		return "", extractErr
	}
	if ffmpegBin == "" {
		return "", fmt.Errorf("no bundled ffmpeg")
	}
	return ffmpegBin, nil
}

func (bundledLocator) FFprobePath() (string, error) {
	extractOnce.Do(extractBundled)
	if extractErr != nil {
		return "", extractErr
	}
	if ffprobeBin == "" {
		return "", fmt.Errorf("no bundled ffprobe")
	}
	return ffprobeBin, nil
}

func extractBundled() {
	tag := fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH)
	base := filepath.Join("embed", "ffmpeg", tag)
	ffmpegData, err1 := os.ReadFile(filepath.Join(base, "ffmpeg"))
	probeData, err2 := os.ReadFile(filepath.Join(base, "ffprobe"))
	if err1 != nil || err2 != nil {
		extractErr = fmt.Errorf("bundled binaries not present for %s", tag)
		return
	}
	dir, err := os.MkdirTemp("", "lerobot-go-ffmpeg-*")
	if err != nil {
		extractErr = err
		return
	}
	ffmpegPath := filepath.Join(dir, "ffmpeg")
	probePath := filepath.Join(dir, "ffprobe")
	if err := os.WriteFile(ffmpegPath, ffmpegData, 0o755); err != nil {
		extractErr = err
		return
	}
	if err := os.WriteFile(probePath, probeData, 0o755); err != nil {
		extractErr = err
		return
	}
	ffmpegBin = ffmpegPath
	ffprobeBin = probePath
}
