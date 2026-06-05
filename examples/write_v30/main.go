// Write a minimal LeRobot v3.0 dataset via staging + merge.
//
// Run from module root:
//
//	go run ./examples/write_v30
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/ioai-tech/lerobot-go/lerobot"
)

func main() {
	ctx := context.Background()
	root, err := os.MkdirTemp("", "lerobot-go-example-*")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(root) }()

	stagingRoot := filepath.Join(root, "staging")
	out := filepath.Join(root, "dataset")
	features := map[string]lerobot.FeatureSpec{
		"observation.state": {DType: "float32", Shape: []int{2}},
		"action":            {DType: "float32", Shape: []int{2}},
	}

	for ep, task := range []string{"pick", "place"} {
		dir := filepath.Join(stagingRoot, fmt.Sprintf("ep_%06d", ep))
		w, err := lerobot.NewStagingWriter(ctx, lerobot.StagingConfig{
			Version:  lerobot.V30,
			Dir:      dir,
			Episode:  ep,
			FPS:      10,
			Features: features,
		})
		if err != nil {
			log.Fatal(err)
		}
		for i := 0; i < 3; i++ {
			if err := w.AddFrame(ctx, lerobot.Frame{
				Task: task,
				Values: map[string]any{
					"observation.state": []float32{float32(i), float32(i + 1)},
					"action":            []float32{0.1, 0.2},
				},
			}); err != nil {
				log.Fatal(err)
			}
		}
		if _, err := w.SaveEpisode(ctx); err != nil {
			log.Fatal(err)
		}
	}

	if err := lerobot.Merge(ctx, lerobot.MergeConfig{
		Version:     lerobot.V30,
		StagingRoot: stagingRoot,
		OutputRoot:  out,
		FPS:         10,
		Features:    features,
		Stats:       lerobot.StatsSampled,
	}); err != nil {
		log.Fatal(err)
	}

	insp := lerobot.NewInspector()
	report, err := insp.Validate(ctx, out)
	if err != nil {
		log.Fatal(err)
	}
	if !report.OK {
		log.Fatalf("validation failed: %v", report.Errors)
	}
	fmt.Printf("OK: wrote v3.0 dataset at %s (%s)\n", out, report.Summary)
}
