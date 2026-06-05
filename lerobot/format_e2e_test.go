package lerobot_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/formatcheck"
	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/lerobot"
)

func TestWriteDatasetFormats(t *testing.T) {
	outRoot := e2eOutputRoot(t)
	v21Out := filepath.Join(outRoot, "v21")
	v30Out := filepath.Join(outRoot, "v30")
	_ = os.RemoveAll(v21Out)
	_ = os.RemoveAll(v30Out)

	ctx := context.Background()
	features := map[string]lerobot.FeatureSpec{
		"observation.state": {DType: "float32", Shape: []int{2}},
		"action":            {DType: "float32", Shape: []int{2}},
	}
	episodes := []struct {
		task   string
		frames int
	}{
		{"pick cube", 3},
		{"place cube", 4},
	}

	if err := writeDataset(ctx, lerobot.CreateConfig{
		Version: lerobot.V21, Root: v21Out, FPS: 10, RobotType: "test_robot", Features: features,
	}, episodes); err != nil {
		t.Fatalf("v2.1 write: %v", err)
	}
	if err := writeDataset(ctx, lerobot.CreateConfig{
		Version: lerobot.V30, Root: v30Out, FPS: 10, RobotType: "test_robot", Features: features,
	}, episodes); err != nil {
		t.Fatalf("v3.0 write: %v", err)
	}

	v21Rep, err := formatcheck.Validate(v21Out)
	if err != nil {
		t.Fatal(err)
	}
	v30Rep, err := formatcheck.Validate(v30Out)
	if err != nil {
		t.Fatal(err)
	}
	if !v21Rep.OK {
		t.Fatalf("v2.1 format invalid:\n  %s", strings.Join(v21Rep.Errors, "\n  "))
	}
	if !v30Rep.OK {
		t.Fatalf("v3.0 format invalid:\n  %s", strings.Join(v30Rep.Errors, "\n  "))
	}

	t.Logf("wrote datasets under %s", outRoot)
	if v21Rep.Version != meta.CodebaseV21 {
		t.Fatalf("v21 dir has version %s", v21Rep.Version)
	}
	if v30Rep.Version != meta.CodebaseV30 {
		t.Fatalf("v30 dir has version %s", v30Rep.Version)
	}
	t.Log("v2.1:", v21Rep.Summary)
	t.Log("v3.0:", v30Rep.Summary)
	logFileTree(t, v21Out, "v2.1")
	logFileTree(t, v30Out, "v3.0")

	inspect, err := lerobot.NewInspector().Validate(ctx, v21Out)
	if err != nil || !inspect.OK {
		t.Fatalf("inspector v2.1: %+v err=%v", inspect, err)
	}
	inspect, err = lerobot.NewInspector().Validate(ctx, v30Out)
	if err != nil || !inspect.OK {
		t.Fatalf("inspector v3.0: %+v err=%v", inspect, err)
	}
}

func writeDataset(ctx context.Context, cfg lerobot.CreateConfig, episodes []struct {
	task   string
	frames int
}) error {
	ds, err := lerobot.Create(ctx, cfg)
	if err != nil {
		return err
	}
	for _, ep := range episodes {
		for i := 0; i < ep.frames; i++ {
			if err := ds.AddFrame(ctx, lerobot.Frame{
				Task: ep.task,
				Values: map[string]any{
					"observation.state": []float32{float32(i), float32(i) + 0.5},
					"action":            []float32{0.1 * float32(i+1), 0.2},
				},
			}); err != nil {
				return err
			}
		}
		if err := ds.SaveEpisode(ctx); err != nil {
			return err
		}
	}
	return ds.Finalize(ctx)
}

func e2eOutputRoot(t *testing.T) string {
	if p := os.Getenv("LEROBOT_E2E_OUT"); p != "" {
		return p
	}
	root := moduleRoot(t)
	return filepath.Join(root, "testdata", "output")
}

func moduleRoot(t *testing.T) string {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("go.mod not found")
		}
		wd = parent
	}
}

func logFileTree(t *testing.T, root, label string) {
	t.Helper()
	var lines []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		lines = append(lines, rel)
		return nil
	})
	t.Logf("%s files (%d):", label, len(lines))
	for _, line := range lines {
		t.Logf("  %s", line)
	}
}

func TestV21FormatCheckDetails(t *testing.T) {
	out := filepath.Join(e2eOutputRoot(t), "v21")
	if _, err := os.Stat(out); err != nil {
		t.Skip("run TestWriteDatasetFormats first")
	}
	rep, err := formatcheck.Validate(out)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Version != meta.CodebaseV21 {
		t.Fatalf("version=%s", rep.Version)
	}
	if !rep.OK {
		t.Fatalf("errors: %v", rep.Errors)
	}
}

func TestV30FormatCheckDetails(t *testing.T) {
	out := filepath.Join(e2eOutputRoot(t), "v30")
	if _, err := os.Stat(out); err != nil {
		t.Skip("run TestWriteDatasetFormats first")
	}
	rep, err := formatcheck.Validate(out)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Version != meta.CodebaseV30 {
		t.Fatalf("version=%s", rep.Version)
	}
	if !rep.OK {
		t.Fatalf("errors: %v", rep.Errors)
	}
	strict, err := formatcheck.ValidateWithOptions(out, formatcheck.Options{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strict.OK {
		t.Fatalf("strict errors: %v", strict.Errors)
	}
	if len(strict.Warnings) != 0 {
		t.Fatalf("strict warnings: %v", strict.Warnings)
	}
}
