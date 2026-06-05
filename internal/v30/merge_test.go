package v30_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/parquetx"
	v30 "github.com/ioai-tech/lerobot-go/internal/v30"
)

func TestMergeMultiEpisodeParquetAppend(t *testing.T) {
	dir := t.TempDir()
	features := map[string]meta.FeatureSpec{
		"observation.state": {DType: "float32", Shape: []int{2}},
		"action":            {DType: "float32", Shape: []int{2}},
	}
	ctx := context.Background()

	for epIdx, task := range []string{"pick", "place"} {
		staging := filepath.Join(dir, fmt.Sprintf("ep_%06d", epIdx))
		w, err := v30.NewStagingWriter(v30.StagingConfig{
			Dir: staging, Episode: epIdx, FPS: 10, Features: features, UseVideos: false,
		})
		if err != nil {
			t.Fatal(err)
		}
		frames := 2 + epIdx
		for i := 0; i < frames; i++ {
			if err := w.AddFrame(ctx, map[string]any{
				"task":              task,
				"observation.state": []float32{float32(i), float32(epIdx)},
				"action":            []float32{0.1, 0.2},
			}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := w.SaveEpisode(ctx); err != nil {
			t.Fatal(err)
		}
	}

	out := filepath.Join(dir, "dataset")
	if err := v30.Merge(ctx, v30.MergeConfig{
		StagingRoot: dir,
		OutputRoot:  out,
		FPS:         10,
		Features:    features,
	}); err != nil {
		t.Fatal(err)
	}

	dataPath := filepath.Join(out, meta.DataPath(0, 0))
	rows, err := parquetx.TableNumRows(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	if rows != 5 {
		t.Fatalf("expected 5 merged rows, got %d", rows)
	}

	tasksPath := filepath.Join(out, meta.DefaultTasksPath)
	if _, err := parquetx.TableNumRows(tasksPath); err != nil {
		t.Fatalf("tasks.parquet unreadable: %v", err)
	}

	episodesPath := filepath.Join(out, meta.EpisodesMetaPath(0, 0))
	epRows, err := parquetx.TableNumRows(episodesPath)
	if err != nil {
		t.Fatalf("episodes parquet unreadable: %v", err)
	}
	if epRows != 2 {
		t.Fatalf("expected 2 episode meta rows, got %d", epRows)
	}

	tbl, err := parquetx.ReadTable(ctx, dataPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	indices, err := parquetx.ExtractInt64Column(tbl, "index")
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{0, 1, 2, 3, 4}
	for i, v := range want {
		if indices[i] != v {
			t.Fatalf("index[%d]=%d want %d", i, indices[i], v)
		}
	}

	episodes, err := parquetx.ReadEpisodesMeta(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 2 {
		t.Fatalf("episodes=%d", len(episodes))
	}
	if episodes[0].DatasetFromIndex != 0 || episodes[0].DatasetToIndex != 2 {
		t.Fatalf("episode0 dataset range=%d:%d", episodes[0].DatasetFromIndex, episodes[0].DatasetToIndex)
	}
	if episodes[1].DatasetFromIndex != 2 || episodes[1].DatasetToIndex != 5 {
		t.Fatalf("episode1 dataset range=%d:%d", episodes[1].DatasetFromIndex, episodes[1].DatasetToIndex)
	}
	if episodes[1].DataChunkIndex != 0 || episodes[1].DataFileIndex != 0 {
		t.Fatalf("episode1 data location=%d/%d", episodes[1].DataChunkIndex, episodes[1].DataFileIndex)
	}

	statsData, err := os.ReadFile(filepath.Join(out, meta.StatsPath))
	if err != nil {
		t.Fatal(err)
	}
	var statsJSON map[string]map[string][]float64
	if err := json.Unmarshal(statsData, &statsJSON); err != nil {
		t.Fatal(err)
	}
	if got := statsJSON["task_index"]["max"][0]; got != 1 {
		t.Fatalf("task_index max=%v want 1", got)
	}
	if got := statsJSON["index"]["max"][0]; got != 4 {
		t.Fatalf("index max=%v want 4", got)
	}
}

func TestMergeRotatesDataFilesBeforeAssigningEpisodeMetadata(t *testing.T) {
	dir := t.TempDir()
	features := map[string]meta.FeatureSpec{
		"observation.state": {DType: "float32", Shape: []int{600_000}},
		"action":            {DType: "float32", Shape: []int{1}},
	}
	ctx := context.Background()

	row := make([]float32, 600_000)
	for i := range row {
		row[i] = float32(i % 13)
	}

	for epIdx, task := range []string{"pick", "place"} {
		staging := filepath.Join(dir, fmt.Sprintf("ep_%06d", epIdx))
		w, err := v30.NewStagingWriter(v30.StagingConfig{
			Dir: staging, Episode: epIdx, FPS: 10, Features: features, UseVideos: false,
		})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			if err := w.AddFrame(ctx, map[string]any{
				"task":              task,
				"observation.state": row,
				"action":            float32(epIdx),
			}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := w.SaveEpisode(ctx); err != nil {
			t.Fatal(err)
		}
	}

	out := filepath.Join(dir, "dataset")
	if err := v30.Merge(ctx, v30.MergeConfig{
		StagingRoot:    dir,
		OutputRoot:     out,
		FPS:            10,
		Features:       features,
		DataFileSizeMB: 1,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(out, meta.DataPath(0, 0))); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, meta.DataPath(0, 1))); err != nil {
		t.Fatal(err)
	}

	episodes, err := parquetx.ReadEpisodesMeta(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 2 {
		t.Fatalf("episodes=%d", len(episodes))
	}
	if episodes[0].DataFileIndex != 0 {
		t.Fatalf("episode0 data file=%d want 0", episodes[0].DataFileIndex)
	}
	if episodes[1].DataFileIndex != 1 {
		t.Fatalf("episode1 data file=%d want 1", episodes[1].DataFileIndex)
	}
	if episodes[1].DatasetFromIndex != 2 || episodes[1].DatasetToIndex != 4 {
		t.Fatalf("episode1 dataset range=%d:%d", episodes[1].DatasetFromIndex, episodes[1].DatasetToIndex)
	}
}
