package v30_test

import (
	"context"
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/parquetx"
	v30 "github.com/ioai-tech/lerobot-go/internal/v30"
	"github.com/ioai-tech/lerobot-go/internal/video"
)

func TestStreamingStagingFlushesParquetChunks(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	t.Setenv("LEROBOT_STREAMING_FLUSH_FRAMES", "2")
	dir := filepath.Join(t.TempDir(), "ep_000000")
	features := map[string]meta.FeatureSpec{
		"observation.images.cam": {DType: "video", Shape: []int{4, 4, 3}},
		"observation.state":      {DType: "float32", Shape: []int{2}},
		"action":                 {DType: "float32", Shape: []int{2}},
	}
	w, err := v30.NewStagingWriter(v30.StagingConfig{
		Dir: dir, Episode: 0, FPS: 10, Features: features, UseVideos: true, Streaming: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	frameRGB := tinyRGB24(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := w.AddFrame(ctx, map[string]any{
			"task": "pick",
			"observation.images.cam": video.VideoFrameRGB24{
				Data: frameRGB, Width: 4, Height: 4,
			},
			"observation.state": []float32{float32(i), 1},
			"action":            []float32{0.1, 0.2},
		}); err != nil {
			t.Fatal(err)
		}
	}
	ep, err := w.SaveEpisode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ep.Length != 5 {
		t.Fatalf("length=%d want 5", ep.Length)
	}
	pq := filepath.Join(dir, "frames.parquet")
	rows, err := parquetx.TableNumRows(pq)
	if err != nil {
		t.Fatal(err)
	}
	if rows != 5 {
		t.Fatalf("rows=%d want 5", rows)
	}
	tbl, err := parquetx.ReadTable(ctx, pq, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	frameIndex, err := parquetx.ExtractInt64Column(tbl, "frame_index")
	if err != nil {
		t.Fatal(err)
	}
	for i, got := range frameIndex {
		if got != int64(i) {
			t.Fatalf("frame_index[%d]=%d", i, got)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "videos", "observation.images.cam.mp4")); err != nil {
		t.Fatalf("video missing: %v", err)
	}
	if ep.Videos["observation.images.cam"] != filepath.Join("videos", "observation.images.cam.mp4") {
		t.Fatalf("videos path=%q want relative staging path", ep.Videos["observation.images.cam"])
	}
}

func tinyRGB24(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 40), G: uint8(y * 40), B: 128, A: 255})
		}
	}
	out := make([]byte, 4*4*3)
	i := 0
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			c := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			out[i] = c.R
			out[i+1] = c.G
			out[i+2] = c.B
			i += 3
		}
	}
	return out
}
