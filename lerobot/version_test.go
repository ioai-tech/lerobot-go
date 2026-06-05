package lerobot_test

import (
	"context"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/lerobot"
)

func TestV21VersionNotTreatedAsUnset(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	ds, err := lerobot.Create(ctx, lerobot.CreateConfig{
		Version: lerobot.V21,
		Root:    dir,
		FPS:     10,
		Features: map[string]lerobot.FeatureSpec{
			"action": {DType: "float32", Shape: []int{2}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ds.AddFrame(ctx, lerobot.Frame{
		Task: "t", Values: map[string]any{"action": []float32{0.5, 0.1}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ds.SaveEpisode(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ds.Finalize(ctx); err != nil {
		t.Fatal(err)
	}
	info, err := meta.LoadInfo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.CodebaseVersion != meta.CodebaseV21 {
		t.Fatalf("got %s want v2.1", info.CodebaseVersion)
	}
}
