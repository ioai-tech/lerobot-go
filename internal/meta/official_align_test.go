package meta_test

import (
	"encoding/json"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/meta"
)

// Field contracts from lerobot v0.3.3 (v2.1) and v0.5.1 (v3.0) create_empty_dataset_info().
func TestNewDatasetInfoMatchesOfficialTemplates(t *testing.T) {
	features := map[string]meta.FeatureSpec{
		"action": {DType: "float32", Shape: []int{2}},
	}

	v21 := meta.NewDatasetInfo(meta.CodebaseV21, 30, features, true, "robot")
	if v21.DataPath != meta.V21DefaultDataPathTemplate {
		t.Fatalf("v2.1 data_path: got %q", v21.DataPath)
	}
	if v21.VideoPath == nil || *v21.VideoPath != meta.V21DefaultVideoPathTemplate {
		t.Fatalf("v2.1 video_path: got %v", v21.VideoPath)
	}
	if v21.TotalChunks == nil || v21.TotalVideos == nil {
		t.Fatal("v2.1 expects total_chunks and total_videos pointers")
	}
	for _, key := range []string{"timestamp", "frame_index", "episode_index", "index", "task_index"} {
		if _, ok := v21.Features[key]; !ok {
			t.Fatalf("v2.1 missing default feature %q", key)
		}
	}

	v30 := meta.NewDatasetInfo(meta.CodebaseV30, 30, features, true, "robot")
	if v30.DataPath != meta.V30DefaultDataPathTemplate {
		t.Fatalf("v3.0 data_path: got %q", v30.DataPath)
	}
	if v30.VideoPath == nil || *v30.VideoPath != meta.V30DefaultVideoPathTemplate {
		t.Fatalf("v3.0 video_path: got %v", v30.VideoPath)
	}
	if v30.TotalChunks != nil || v30.TotalVideos != nil {
		t.Fatal("v3.0 info must not include total_chunks/total_videos at create")
	}
	if v30.DataFilesSizeInMB != meta.DefaultDataFileSizeInMB || v30.VideoFilesSizeInMB != meta.DefaultVideoFileSizeInMB {
		t.Fatal("v3.0 chunk size limits mismatch")
	}
}

func TestV21EpisodeChunkPaths(t *testing.T) {
	info := meta.NewDatasetInfo(meta.CodebaseV21, 30, nil, true, "")
	info.ChunksSize = 1000

	if got := meta.V21DataPathFromInfo(info, 0); got != "data/chunk-000/episode_000000.parquet" {
		t.Fatalf("ep0 data: %q", got)
	}
	if got := meta.V21DataPathFromInfo(info, 1000); got != "data/chunk-001/episode_001000.parquet" {
		t.Fatalf("ep1000 data: %q", got)
	}
	vid := meta.V21VideoPathFromInfo(info, "observation.images.cam", 1000)
	want := "videos/chunk-001/observation.images.cam/episode_001000.mp4"
	if vid != want {
		t.Fatalf("ep1000 video: got %q want %q", vid, want)
	}
}

func TestFeatureSpecRoundTripsUnknownFields(t *testing.T) {
	spec := meta.FeatureSpec{
		DType: "video",
		Shape: []int{480, 640, 3},
		Names: []string{"height", "width", "channel"},
		Info: map[string]any{
			"video.codec": "h264",
		},
		Extra: map[string]any{
			"description": "front camera",
			"fps":         float64(30),
		},
	}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	var got meta.FeatureSpec
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !spec.Equal(got) {
		t.Fatalf("roundtrip mismatch:\nwant=%s\ngot=%s", mustJSON(t, spec), mustJSON(t, got))
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
