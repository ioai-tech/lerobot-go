package parquetx

import (
	"path/filepath"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/stats"
)

func TestWriteEpisodesParquetImageStat311(t *testing.T) {
	root := t.TempDir()
	key := "observation.images.cam"
	features := map[string]meta.FeatureSpec{
		key: {DType: "video", Shape: []int{64, 64, 3}},
	}
	img := stats.ImageStat311{{{0.4}}, {{0.2}}, {{0.1}}}
	err := WriteEpisodesParquet(root, []EpisodeMetaInput{{
		EpisodeIndex: 0,
		Tasks:        []string{"pick"},
		Length:       3,
		Fields: map[string]any{
			"data/chunk_index":   0,
			"data/file_index":    0,
			"dataset_from_index": int64(0),
			"dataset_to_index":   int64(3),
		},
		Stats: stats.EpisodeStats{
			key: {
				"min": img, "max": img, "mean": img, "std": img,
				"count": []int64{3},
			},
		},
		Features: features,
	}}, 100)
	if err != nil {
		t.Fatalf("WriteEpisodesParquet: %v", err)
	}
	path := filepath.Join(root, meta.EpisodesMetaPath(0, 0))
	if _, err := TableNumRows(path); err != nil {
		t.Fatalf("read episodes parquet: %v", err)
	}
}
