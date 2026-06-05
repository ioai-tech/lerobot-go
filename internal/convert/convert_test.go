package convert_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/convert"
	"github.com/ioai-tech/lerobot-go/internal/formatcheck"
	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/lerobot"
)

func TestV21ToV30PreservesFeatureMetadataAndTasks(t *testing.T) {
	ctx := context.Background()
	input := filepath.Join(t.TempDir(), "v21")
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
	writeDataset(t, ctx, input, lerobot.CreateConfig{
		Version:  lerobot.V21,
		Root:     input,
		FPS:      10,
		Features: features,
	}, []episodeSpec{{task: "pick", frames: 2}, {task: "place", frames: 3}})

	output := filepath.Join(t.TempDir(), "v30")
	if err := convert.Run(ctx, convert.Config{
		Input:  input,
		Output: output,
		From:   lerobot.V21,
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
	if info.Features["observation.state"].Extra["description"] != "robot state" {
		t.Fatalf("observation.state description lost: %+v", info.Features["observation.state"])
	}
	if info.Features["action"].Extra["description"] != "robot action" {
		t.Fatalf("action description lost: %+v", info.Features["action"])
	}
}

type episodeSpec struct {
	task   string
	frames int
}

func writeDataset(t *testing.T, ctx context.Context, root string, cfg lerobot.CreateConfig, episodes []episodeSpec) {
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
