package v30

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/video"
)

// Regression: when AddFrame receives stray RGB values for a feature that is
// later finalized via SetH264Remux, the RawRGBEncoder used to flush a black
// mp4 over the remuxed file on Close(), producing all-black exported videos.
func TestStagingRemuxSurvivesStrayRGBFrames(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	key := "observation.images.cam"
	const w0, h0, n = 64, 64, 5
	aus := genTestAUs(t, w0, h0, n)

	w, err := NewStagingWriter(StagingConfig{
		Dir:       dir,
		Episode:   0,
		FPS:       10,
		UseVideos: true,
		H264Remux: true,
		Features: map[string]meta.FeatureSpec{
			key: {DType: "video", Shape: []int{h0, w0, 3}},
		},
	})
	if err != nil {
		t.Fatalf("NewStagingWriter: %v", err)
	}
	black := video.VideoFrameRGB24{Width: w0, Height: h0, Data: make([]byte, w0*h0*3)}
	for i := 0; i < n; i++ {
		if err := w.AddFrame(context.Background(), map[string]any{"task": "t", key: black}); err != nil {
			t.Fatalf("AddFrame: %v", err)
		}
	}
	if err := w.SetH264Remux(context.Background(), map[string][][]byte{key: aus}); err != nil {
		t.Fatalf("SetH264Remux: %v", err)
	}
	ep, err := w.SaveEpisode(context.Background())
	if err != nil {
		t.Fatalf("SaveEpisode: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	out := filepath.Join(dir, ep.Videos[key])
	assertVideoFramesNotBlack(t, out, n)
}

// genTestAUs encodes a short colourful test pattern with all-IDR frames and
// splits the Annex-B stream into access units (one VCL NAL per frame).
func genTestAUs(t *testing.T, width, height, frames int) [][]byte {
	t.Helper()
	cmd := exec.Command("ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc=size=%dx%d:rate=10", width, height),
		"-frames:v", strconv.Itoa(frames),
		"-c:v", "libx264", "-g", "1", "-keyint_min", "1",
		"-pix_fmt", "yuv420p",
		"-f", "h264", "pipe:1",
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		t.Fatalf("generate h264: %v", err)
	}
	aus := splitAnnexBAUs(buf.Bytes())
	if len(aus) != frames {
		t.Fatalf("expected %d AUs, got %d", frames, len(aus))
	}
	return aus
}

// splitAnnexBAUs closes an access unit after each VCL NAL (types 1-5).
func splitAnnexBAUs(data []byte) [][]byte {
	var starts []int
	for i := 0; i+3 < len(data); i++ {
		if data[i] == 0 && data[i+1] == 0 && (data[i+2] == 1 || (data[i+2] == 0 && data[i+3] == 1)) {
			starts = append(starts, i)
			if data[i+2] == 0 {
				i += 3
			} else {
				i += 2
			}
		}
	}
	var aus [][]byte
	auStart := 0
	for idx, pos := range starts {
		sc := 3
		if data[pos+2] == 0 {
			sc = 4
		}
		nalType := data[pos+sc] & 0x1f
		end := len(data)
		if idx+1 < len(starts) {
			end = starts[idx+1]
		}
		if nalType >= 1 && nalType <= 5 {
			aus = append(aus, data[auStart:end])
			auStart = end
		}
	}
	return aus
}

func assertVideoFramesNotBlack(t *testing.T, path string, wantFrames int) {
	t.Helper()
	probe := exec.Command("ffprobe",
		"-v", "error", "-select_streams", "v:0",
		"-count_packets", "-show_entries", "stream=nb_read_packets",
		"-of", "csv=p=0", path,
	)
	out, err := probe.Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	got, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("parse packet count %q: %v", out, err)
	}
	if got != wantFrames {
		t.Fatalf("packet count = %d, want %d", got, wantFrames)
	}
	dec := exec.Command("ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-i", path, "-frames:v", "1",
		"-f", "rawvideo", "-pix_fmt", "rgb24", "pipe:1",
	)
	var raw bytes.Buffer
	dec.Stdout = &raw
	if err := dec.Run(); err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	maxVal := byte(0)
	for _, b := range raw.Bytes() {
		if b > maxVal {
			maxVal = b
		}
	}
	if maxVal < 32 {
		t.Fatalf("decoded frame is black (max pixel %d); remux output was clobbered", maxVal)
	}
}
