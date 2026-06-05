package aggregate_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/aggregate"
	"github.com/ioai-tech/lerobot-go/internal/formatcheck"
	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/parquetx"
	"github.com/ioai-tech/lerobot-go/lerobot"
)

func TestRunMergeV30PreservesFeatureMetadataAndTaskMapping(t *testing.T) {
	ctx := context.Background()
	features := map[string]lerobot.FeatureSpec{
		"observation.state": {
			DType: "float32",
			Shape: []int{2},
			Names: []string{"joint_0", "joint_1"},
			Extra: map[string]any{
				"description": "robot state",
			},
		},
		"action": {
			DType: "float32",
			Shape: []int{2},
			Extra: map[string]any{
				"description": "robot action",
			},
		},
	}

	rootA := filepath.Join(t.TempDir(), "a")
	writeAggregateDataset(t, ctx, rootA, lerobot.CreateConfig{
		Version:  lerobot.V30,
		Root:     rootA,
		FPS:      10,
		Features: features,
	}, []episodeSpec{{task: "pick", frames: 2}})

	rootB := filepath.Join(t.TempDir(), "b")
	writeAggregateDataset(t, ctx, rootB, lerobot.CreateConfig{
		Version:  lerobot.V30,
		Root:     rootB,
		FPS:      10,
		Features: features,
	}, []episodeSpec{{task: "place", frames: 3}})

	output := filepath.Join(t.TempDir(), "merged")
	if err := aggregate.Run(ctx, aggregate.Config{
		Inputs: []string{rootA, rootB},
		Output: output,
		To:     lerobot.V30,
	}); err != nil {
		t.Fatal(err)
	}

	rep, err := formatcheck.ValidateWithOptions(output, formatcheck.Options{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK {
		t.Fatalf("format errors: %v", rep.Errors)
	}

	info, err := meta.LoadInfo(output)
	if err != nil {
		t.Fatal(err)
	}
	if info.TotalEpisodes != 2 || info.TotalFrames != 5 || info.TotalTasks != 2 {
		t.Fatalf("merged info=%+v", info)
	}
	if info.Features["observation.state"].Extra["description"] != "robot state" {
		t.Fatalf("feature metadata lost: %+v", info.Features["observation.state"])
	}

	taskMap, err := parquetx.ReadTasksParquet(ctx, filepath.Join(output, meta.DefaultTasksPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(taskMap) != 2 || taskMap["pick"] == taskMap["place"] {
		t.Fatalf("task map=%v", taskMap)
	}
}

type episodeSpec struct {
	task   string
	frames int
}

func writeAggregateDataset(t *testing.T, ctx context.Context, root string, cfg lerobot.CreateConfig, episodes []episodeSpec) {
	t.Helper()
	ds, err := lerobot.Create(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, ep := range episodes {
		for i := 0; i < ep.frames; i++ {
			if err := ds.AddFrame(ctx, lerobot.Frame{
				Task: ep.task,
				Values: map[string]any{
					"observation.state": []float32{float32(i), float32(i + 1)},
					"action":            []float32{float32(i), 0},
				},
			}); err != nil {
				t.Fatal(err)
			}
		}
		if err := ds.SaveEpisode(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := ds.Finalize(ctx); err != nil {
		t.Fatal(err)
	}
}
