package v30

import (
	"context"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/meta"
)

func TestSaveEpisodeFailsWithoutVideoFrames(t *testing.T) {
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
	if err := w.AddFrame(context.Background(), map[string]any{"task": "t"}); err != nil {
		t.Fatalf("AddFrame: %v", err)
	}
	if _, err := w.SaveEpisode(context.Background()); err == nil {
		t.Fatal("expected SaveEpisode to fail without video output")
	}
}

func TestSaveEpisodeFailsWithEmptyRemuxTrack(t *testing.T) {
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
	if err := w.AddFrame(context.Background(), map[string]any{"task": "t"}); err != nil {
		t.Fatalf("AddFrame: %v", err)
	}
	if err := w.SetH264Remux(context.Background(), map[string][][]byte{key: {}}); err != nil {
		t.Fatalf("SetH264Remux: %v", err)
	}
	if _, err := w.SaveEpisode(context.Background()); err == nil {
		t.Fatal("expected SaveEpisode to fail with empty remux track")
	}
}
