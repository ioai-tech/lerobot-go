package v21_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/formatcheck"
	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/parquetx"
	v21 "github.com/ioai-tech/lerobot-go/internal/v21"
)

func TestMergeV21MultiEpisode(t *testing.T) {
	dir := t.TempDir()
	features := map[string]meta.FeatureSpec{
		"observation.state": {DType: "float32", Shape: []int{2}},
		"action":            {DType: "float32", Shape: []int{2}},
	}
	ctx := context.Background()

	for epIdx, task := range []string{"pick", "place"} {
		staging := filepath.Join(dir, fmt.Sprintf("ep_%06d", epIdx))
		w, err := v21.NewStagingWriter(v21.StagingConfig{
			Dir: staging, Episode: epIdx, FPS: 10, Features: features, UseVideos: false,
		})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2+epIdx; i++ {
			if err := w.AddFrame(ctx, map[string]any{
				"task":              task,
				"observation.state": []float32{1, 2},
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
	if err := v21.Merge(ctx, v21.MergeConfig{
		StagingRoot: dir, OutputRoot: out, FPS: 10, Features: features,
	}); err != nil {
		t.Fatal(err)
	}

	rep, err := formatcheck.Validate(out)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK {
		t.Fatalf("format errors: %v", rep.Errors)
	}

	pq0 := filepath.Join(out, meta.V21DataPath(0))
	pq1 := filepath.Join(out, meta.V21DataPath(1))
	n0, _ := parquetx.TableNumRows(pq0)
	n1, _ := parquetx.TableNumRows(pq1)
	if n0 != 2 || n1 != 3 {
		t.Fatalf("rows ep0=%d ep1=%d", n0, n1)
	}
	tbl, err := parquetx.ReadTable(ctx, pq1, nil)
	if err != nil {
		t.Fatal(err)
	}
	indices, err := parquetx.ExtractInt64Column(tbl, "index")
	tbl.Release()
	if err != nil || indices[0] != 2 {
		t.Fatalf("episode1 index start=%v err=%v", indices, err)
	}

	info, err := meta.LoadInfo(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.TotalVideos == nil || *info.TotalVideos != 0 {
		t.Fatalf("total_videos=%v want 0", info.TotalVideos)
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
