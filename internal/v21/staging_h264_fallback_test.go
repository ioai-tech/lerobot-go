package v21

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/manifest"
	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/video"
)

func TestStagingH264DecodeFallbackPopulatesVideos(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	key := "observation.images.cam"
	w, err := NewStagingWriter(StagingConfig{
		Dir:       dir,
		Episode:   0,
		FPS:       10,
		UseVideos: true,
		H264Remux: true,
		Features: map[string]meta.FeatureSpec{
			key: {DType: "video", Shape: []int{4, 4, 3}},
		},
	})
	if err != nil {
		t.Fatalf("NewStagingWriter: %v", err)
	}
	frame := video.VideoFrameRGB24{
		Width: 4, Height: 4,
		Data: make([]byte, 4*4*3),
	}
	for i := 0; i < 3; i++ {
		if err := w.AppendRGBVideoFrame(context.Background(), key, frame); err != nil {
			t.Fatalf("AppendRGBVideoFrame: %v", err)
		}
		if err := w.AddFrame(context.Background(), map[string]any{"task": "t"}); err != nil {
			t.Fatalf("AddFrame: %v", err)
		}
	}
	ep, err := w.SaveEpisode(context.Background())
	if err != nil {
		t.Fatalf("SaveEpisode: %v", err)
	}
	if len(ep.Videos) == 0 || ep.Videos[key] == "" {
		t.Fatalf("expected videos map entry, got %#v", ep.Videos)
	}
	out := manifest.StagingMediaPath(dir, ep.Videos[key])
	fi, err := os.Stat(out)
	if err != nil {
		t.Fatalf("missing staged mp4: %v", err)
	}
	if fi.Size() == 0 {
		t.Fatal("empty staged mp4")
	}
	rel := filepath.Base(out)
	if rel != "observation.images.cam.mp4" {
		t.Fatalf("unexpected video rel basename: %s", rel)
	}
}
