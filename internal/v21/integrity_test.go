package v21

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/meta"
)

func TestValidateOutputIntegrityParquetOK(t *testing.T) {
	root := t.TempDir()
	info := meta.NewDatasetInfo(meta.CodebaseV21, 30, map[string]meta.FeatureSpec{
		"observation.state": {DType: "float32", Shape: []int{2}},
	}, false, "test")
	info.TotalEpisodes = 1
	pq := filepath.Join(root, meta.V21DataPathFromInfo(info, 0))
	if err := os.MkdirAll(filepath.Dir(pq), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pq, []byte("par1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateOutputIntegrity(root, info, info.Features); err != nil {
		t.Fatalf("expected ok: %v", err)
	}
}

func TestValidateOutputIntegrityMissingParquet(t *testing.T) {
	root := t.TempDir()
	info := meta.NewDatasetInfo(meta.CodebaseV21, 30, map[string]meta.FeatureSpec{
		"observation.state": {DType: "float32", Shape: []int{2}},
	}, false, "test")
	info.TotalEpisodes = 1
	if err := ValidateOutputIntegrity(root, info, info.Features); err == nil {
		t.Fatal("expected error for missing parquet")
	}
}

func TestValidateOutputIntegrityVideoRequired(t *testing.T) {
	root := t.TempDir()
	features := map[string]meta.FeatureSpec{
		"observation.state":              {DType: "float32", Shape: []int{2}},
		"observation.images.cam":         {DType: "video", Shape: []int{4, 4, 3}},
	}
	info := meta.NewDatasetInfo(meta.CodebaseV21, 30, features, true, "test")
	info.TotalEpisodes = 1
	pq := filepath.Join(root, meta.V21DataPathFromInfo(info, 0))
	if err := os.MkdirAll(filepath.Dir(pq), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pq, []byte("par1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateOutputIntegrity(root, info, features); err == nil {
		t.Fatal("expected error for missing video")
	}
	vid := filepath.Join(root, meta.V21VideoPathFromInfo(info, "observation.images.cam", 0))
	if err := os.MkdirAll(filepath.Dir(vid), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vid, []byte("mp4"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateOutputIntegrity(root, info, features); err != nil {
		t.Fatalf("expected ok with video: %v", err)
	}
}
