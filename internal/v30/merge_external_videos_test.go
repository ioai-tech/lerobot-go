package v30_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/formatcheck"
	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/parquetx"
	"github.com/ioai-tech/lerobot-go/internal/stats"
	v30 "github.com/ioai-tech/lerobot-go/internal/v30"
)

func TestMergeExternalVideosIncludesVisualStats(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}

	const (
		key    = "observation.images.camera_high"
		frames = 8
		fps    = 10
	)
	dir := t.TempDir()
	mp4 := filepath.Join(dir, "src.mp4")
	writeSolidMP4(t, mp4, "red", frames, fps)

	features := map[string]meta.FeatureSpec{
		"observation.state": {DType: "float32", Shape: []int{2}},
		"action":            {DType: "float32", Shape: []int{2}},
		key:                 {DType: "video", Shape: []int{64, 64, 3}},
	}
	ctx := context.Background()
	staging := filepath.Join(dir, "ep_000000")
	w, err := v30.NewStagingWriter(v30.StagingConfig{
		Dir:            staging,
		Episode:        0,
		FPS:            fps,
		Features:       features,
		UseVideos:      true,
		ExternalVideos: true,
		Stats:          stats.Options{Mode: stats.ModeSampled},
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < frames; i++ {
		if err := w.AddFrame(ctx, map[string]any{
			"task":              "pick",
			"observation.state": []float32{float32(i), 0},
			"action":            []float32{0.1, 0.2},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.SetVideoFiles(ctx, map[string]string{key: mp4}); err != nil {
		t.Fatal(err)
	}
	ep, err := w.SaveEpisode(ctx)
	if err != nil {
		t.Fatalf("SaveEpisode: %v", err)
	}
	if ep.Stats[key] == nil {
		t.Fatal("staging episode stats omitted video key")
	}

	out := filepath.Join(dir, "dataset")
	if err := v30.Merge(ctx, v30.MergeConfig{
		StagingRoot: dir,
		OutputRoot:  out,
		FPS:         fps,
		Features:    features,
		Stats:       stats.Options{Mode: stats.ModeSampled},
	}); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(out, meta.StatsPath))
	if err != nil {
		t.Fatal(err)
	}
	var statsJSON map[string]map[string]any
	if err := json.Unmarshal(raw, &statsJSON); err != nil {
		t.Fatal(err)
	}
	cam, ok := statsJSON[key]
	if !ok {
		t.Fatalf("stats.json missing %s; keys=%v", key, keysOf(statsJSON))
	}
	mean, ok := stats.AsImageStat311(cam["mean"])
	if !ok {
		t.Fatalf("stats.json %s mean type %T", key, cam["mean"])
	}
	ch := mean.Channels()
	if len(ch) != 3 || ch[0] < 0.5 {
		t.Fatalf("stats.json mean=%v want R>0.5 in [0,1]", ch)
	}

	info, err := meta.LoadInfo(out)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := info.Features[key]; !ok {
		t.Fatalf("info.json missing %s", key)
	}

	if _, err := parquetx.TableNumRows(filepath.Join(out, meta.EpisodesMetaPath(0, 0))); err != nil {
		t.Fatalf("episodes parquet: %v", err)
	}

	res, err := formatcheck.Validate(out)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("formatcheck: %v", res.Errors)
	}
}

func writeSolidMP4(t *testing.T, path, color string, frames, fps int) {
	t.Helper()
	cmd := exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c="+color+":s=64x64:r="+strconv.Itoa(fps),
		"-frames:v", strconv.Itoa(frames),
		"-an", "-sn",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate mp4: %v\n%s", err, out)
	}
}

func keysOf(m map[string]map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
