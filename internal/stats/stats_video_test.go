package stats

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestComputeEpisodeStatsFromVideo(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	dir := t.TempDir()
	mp4 := filepath.Join(dir, "cam.mp4")
	writeSolidMP4(t, mp4, "red", 8, 10)

	ep := ComputeEpisodeStats(EpisodeInput{
		VideoFiles: map[string]string{"cam": mp4},
		Length:     8,
		Ctx:        context.Background(),
	}, map[string]FeatureDesc{
		"cam": {DType: "video", Shape: []int{64, 64, 3}},
	}, Options{Mode: ModeSampled})

	st := ep["cam"]
	if st == nil {
		t.Fatal("missing cam stats")
	}
	mean, ok := AsImageStat311(st["mean"])
	if !ok {
		t.Fatalf("mean type %T", st["mean"])
	}
	ch := mean.Channels()
	if len(ch) != 3 {
		t.Fatalf("channels=%d", len(ch))
	}
	if ch[0] < 0.5 {
		t.Fatalf("red channel mean=%v want >0.5 (missing or double-scaled stats)", ch[0])
	}
	if ch[0] > 1.0 || ch[1] < 0 || ch[2] < 0 {
		t.Fatalf("channels out of [0,1]: %v", ch)
	}
}

func writeSolidMP4(t *testing.T, path, color string, frames, fps int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c="+color+":s=64x64:r="+itoa(fps),
		"-frames:v", itoa(frames),
		"-an", "-sn",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate mp4: %v\n%s", err, out)
	}
}
