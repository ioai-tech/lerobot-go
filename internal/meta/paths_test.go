package meta_test

import (
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/meta"
)

func TestV21DataPathFromInfo(t *testing.T) {
	info := meta.DatasetInfo{
		DataPath:   "data/chunk-{episode_chunk:03d}/episode_{episode_index:06d}.parquet",
		ChunksSize: 1000,
		VideoPath:  strPtr("videos/chunk-{episode_chunk:03d}/{video_key}/episode_{episode_index:06d}.mp4"),
	}
	got := meta.V21DataPathFromInfo(info, 0)
	want := "data/chunk-000/episode_000000.parquet"
	if got != want {
		t.Fatalf("data path: got %q want %q", got, want)
	}
	vid := meta.V21VideoPathFromInfo(info, "observation.images.cam", 0)
	if vid != "videos/chunk-000/observation.images.cam/episode_000000.mp4" {
		t.Fatalf("video path: got %q", vid)
	}
}

func strPtr(s string) *string { return &s }
