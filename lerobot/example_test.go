package lerobot_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ioai-tech/lerobot-go/lerobot"
)

func ExampleNewStagingWriter_merge() {
	ctx := context.Background()
	root, err := os.MkdirTemp("", "lerobot-example-*")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(root) }()

	stagingRoot := filepath.Join(root, "staging")
	out := filepath.Join(root, "out")
	features := map[string]lerobot.FeatureSpec{
		"observation.state": {DType: "float32", Shape: []int{2}},
		"action":            {DType: "float32", Shape: []int{2}},
	}

	w, err := lerobot.NewStagingWriter(ctx, lerobot.StagingConfig{
		Version: lerobot.V30, Dir: filepath.Join(stagingRoot, "ep_000000"),
		Episode: 0, FPS: 10, Features: features,
	})
	if err != nil {
		panic(err)
	}
	_ = w.AddFrame(ctx, lerobot.Frame{
		Task: "demo",
		Values: map[string]any{
			"observation.state": []float32{1, 2},
			"action":            []float32{0, 0},
		},
	})
	_, _ = w.SaveEpisode(ctx)
	_ = lerobot.Merge(ctx, lerobot.MergeConfig{
		Version: lerobot.V30, StagingRoot: stagingRoot, OutputRoot: out,
		FPS: 10, Features: features,
	})

	insp := lerobot.NewInspector()
	report, _ := insp.Validate(ctx, out)
	fmt.Println(report.OK)
	// Output: true
}

func ExampleNewInspector() {
	ctx := context.Background()
	root, err := os.MkdirTemp("", "lerobot-insp-*")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(root) }()

	// Empty dir is not a valid dataset; inspector reports errors without panicking.
	insp := lerobot.NewInspector()
	report, err := insp.Validate(ctx, root)
	if err != nil {
		panic(err)
	}
	fmt.Println(report.OK)
	// Output: false
}
