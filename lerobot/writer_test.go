package lerobot_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/lerobot"
)

func TestStagingWriterV30(t *testing.T) {
	dir := t.TempDir()
	staging := filepath.Join(dir, "ep_000000")
	ctx := context.Background()
	w, err := lerobot.NewStagingWriter(ctx, lerobot.StagingConfig{
		Version: lerobot.V30,
		Dir:     staging,
		Episode: 0,
		FPS:     10,
		Features: map[string]lerobot.FeatureSpec{
			"observation.state": {DType: "float32", Shape: []int{2}},
			"action":            {DType: "float32", Shape: []int{2}},
		},
		UseVideos: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := w.AddFrame(ctx, lerobot.Frame{
			Task: "pick",
			Values: map[string]any{
				"observation.state": []float32{1, 2},
				"action":            []float32{0.1, 0.2},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	ep, err := w.SaveEpisode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ep.Length != 3 {
		t.Fatalf("length=%d", ep.Length)
	}
	out := filepath.Join(dir, "out")
	if err := lerobot.Merge(ctx, lerobot.MergeConfig{
		Version: lerobot.V30, StagingRoot: dir, OutputRoot: out, FPS: 10,
		Features: map[string]lerobot.FeatureSpec{
			"observation.state": {DType: "float32", Shape: []int{2}},
			"action":            {DType: "float32", Shape: []int{2}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	info, err := meta.LoadInfo(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.TotalEpisodes != 1 || info.TotalFrames != 3 {
		t.Fatalf("info=%+v", info)
	}
	if _, err := os.Stat(filepath.Join(out, "meta/tasks.parquet")); err != nil {
		t.Fatalf("tasks.parquet missing: %v", err)
	}
}

func TestMergeV30MultiEpisode(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	features := map[string]lerobot.FeatureSpec{
		"observation.state": {DType: "float32", Shape: []int{2}},
		"action":            {DType: "float32", Shape: []int{2}},
	}
	for ep, task := range []string{"pick", "place"} {
		staging := filepath.Join(dir, fmt.Sprintf("ep_%06d", ep))
		w, err := lerobot.NewStagingWriter(ctx, lerobot.StagingConfig{
			Version: lerobot.V30, Dir: staging, Episode: ep, FPS: 10, Features: features,
		})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			if err := w.AddFrame(ctx, lerobot.Frame{
				Task: task,
				Values: map[string]any{
					"observation.state": []float32{1, 2},
					"action":            []float32{0.1, 0.2},
				},
			}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := w.SaveEpisode(ctx); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, "out")
	if err := lerobot.Merge(ctx, lerobot.MergeConfig{
		Version: lerobot.V30, StagingRoot: dir, OutputRoot: out, FPS: 10, Features: features,
	}); err != nil {
		t.Fatal(err)
	}
	info, err := meta.LoadInfo(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.TotalEpisodes != 2 || info.TotalFrames != 4 {
		t.Fatalf("info=%+v", info)
	}
}

func TestStagingWriterRejectsVideoFeaturesWithoutUseVideos(t *testing.T) {
	_, err := lerobot.NewStagingWriter(context.Background(), lerobot.StagingConfig{
		Version: lerobot.V30,
		Dir:     filepath.Join(t.TempDir(), "ep_000000"),
		Episode: 0,
		FPS:     10,
		Features: map[string]lerobot.FeatureSpec{
			"observation.images.cam": {DType: "video", Shape: []int{4, 4, 3}},
		},
		UseVideos: false,
	})
	if err == nil {
		t.Fatal("expected error for video features with UseVideos=false")
	}
}

func TestInspectorValidate(t *testing.T) {
	dir := t.TempDir()
	info := meta.NewDatasetInfo(meta.CodebaseV30, 30, map[string]meta.FeatureSpec{
		"action": {DType: "float32", Shape: []int{1}},
	}, false, "test")
	info.TotalEpisodes = 1
	info.TotalFrames = 10
	if err := meta.WriteInfo(dir, info); err != nil {
		t.Fatal(err)
	}
	rep, err := lerobot.NewInspector().Validate(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK {
		t.Fatalf("expected incomplete dataset to fail validation")
	}
	if rep.Info.TotalFrames != 10 {
		t.Fatalf("info=%+v", rep.Info)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
