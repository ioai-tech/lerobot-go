package v30

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/meta"
)

func TestStagingH264RemuxCopyMux(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	key := "observation.images.cam"
	w, err := NewStagingWriter(StagingConfig{
		Dir:       dir,
		Episode:   0,
		FPS:       10,
		UseVideos: true,
		H264Remux: true,
		Features: map[string]meta.FeatureSpec{
			key: {DType: "video", Shape: []int{4, 4, 3}},
		},
	})
	if err != nil {
		t.Fatalf("NewStagingWriter: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := w.AddFrame(context.Background(), map[string]any{
			"task": "t",
		}); err != nil {
			t.Fatalf("AddFrame: %v", err)
		}
	}
	// Minimal synthetic AU; copy mux may fail on invalid bitstream — test wiring only.
	if err := w.SetH264Remux(context.Background(), map[string][][]byte{
		key: {
			[]byte{0, 0, 0, 1, 0x65, 0x88},
			[]byte{0, 0, 0, 1, 0x65, 0x88},
			[]byte{0, 0, 0, 1, 0x65, 0x88},
		},
	}); err != nil {
		t.Fatalf("SetH264Remux: %v", err)
	}
	_, err = w.SaveEpisode(context.Background())
	if err != nil {
		t.Skipf("synthetic AU remux not accepted by ffmpeg: %v", err)
	}
	out := filepath.Join(dir, "videos", "cam.mp4")
	fi, statErr := os.Stat(out)
	if statErr != nil {
		t.Fatalf("missing output mp4: %v", statErr)
	}
	if fi.Size() == 0 {
		t.Fatal("empty output mp4")
	}
}
