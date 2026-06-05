package stats

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestComputeEpisodeStatsImageSampled(t *testing.T) {
	dir := t.TempDir()
	paths := make([]string, 4)
	for i := range paths {
		path := filepath.Join(dir, fmt.Sprintf("frame-%d.png", i))
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		img := image.NewRGBA(image.Rect(0, 0, 8, 8))
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				img.SetRGBA(x, y, color.RGBA{R: uint8(i * 40), G: 10, B: 20, A: 255})
			}
		}
		if err := png.Encode(f, img); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		_ = f.Close()
		paths[i] = path
	}

	ep := ComputeEpisodeStats(EpisodeInput{
		FramePaths: map[string][]string{"cam": paths},
	}, map[string]FeatureDesc{
		"cam": {DType: "image", Shape: []int{8, 8, 3}},
	}, Options{Mode: ModeSampled})

	st := ep["cam"]
	if st == nil {
		t.Fatal("missing cam stats")
	}
	mean, ok := st["mean"].(ImageStat311)
	if !ok {
		t.Fatalf("mean type %T", st["mean"])
	}
	if len(mean) != 3 {
		t.Fatalf("channels=%d", len(mean))
	}
}

func TestComputeEpisodeStatsImageBytesSampled(t *testing.T) {
	frames := make([][]byte, 4)
	for i := range frames {
		img := image.NewRGBA(image.Rect(0, 0, 8, 8))
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				img.SetRGBA(x, y, color.RGBA{R: uint8(i * 40), G: 10, B: 20, A: 255})
			}
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		frames[i] = buf.Bytes()
	}

	ep := ComputeEpisodeStats(EpisodeInput{
		FrameBytes: map[string][][]byte{"cam": frames},
	}, map[string]FeatureDesc{
		"cam": {DType: "image", Shape: []int{8, 8, 3}},
	}, Options{Mode: ModeSampled})

	st := ep["cam"]
	if st == nil {
		t.Fatal("missing cam stats")
	}
	mean, ok := st["mean"].(ImageStat311)
	if !ok {
		t.Fatalf("mean type %T", st["mean"])
	}
	if len(mean) != 3 {
		t.Fatalf("channels=%d", len(mean))
	}
}
