package v21_test

import (
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/parquetx"
	v21 "github.com/ioai-tech/lerobot-go/internal/v21"
)

func TestStagingWriterEmbedsImages(t *testing.T) {
	dir := t.TempDir()
	features := map[string]meta.FeatureSpec{
		"observation.images.cam": {DType: "image", Shape: []int{4, 4, 3}},
	}
	w, err := v21.NewStagingWriter(v21.StagingConfig{
		Dir: dir, Episode: 0, FPS: 10, Features: features, UseVideos: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var buf []byte
	{
		f, _ := os.Create(filepath.Join(dir, "tmp.png"))
		_ = png.Encode(f, img)
		_ = f.Close()
		b, _ := os.ReadFile(filepath.Join(dir, "tmp.png"))
		buf = b
	}
	ctx := context.Background()
	if err := w.AddFrame(ctx, map[string]any{
		"task":                   "pick",
		"observation.images.cam": buf,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.SaveEpisode(ctx); err != nil {
		t.Fatal(err)
	}
	pqPath := filepath.Join(dir, "frames.parquet")
	tbl, err := parquetx.ReadTable(ctx, pqPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	if len(tbl.Schema().FieldIndices("observation.images.cam")) == 0 {
		t.Fatal("image column missing from logical arrow schema")
	}
	if _, err := os.Stat(filepath.Join(dir, "_tmp_frames")); !os.IsNotExist(err) {
		t.Fatalf("image-only staging should not create temp frame dir, err=%v", err)
	}
}
