package video_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/video"
)

func TestGetVideoInfoFromRealDataset(t *testing.T) {
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, "Downloads/lerobot_dataset/data/chunk-000/episode_000000.mp4"),
	}
	var videoPath string
	for _, c := range candidates {
		for _, key := range []string{
			"videos/chunk-000/observation.images.x2w_camera_head_realsense_compressed/episode_000000.mp4",
		} {
			p := filepath.Join(home, "Downloads/lerobot_dataset", key)
			if _, err := os.Stat(p); err == nil {
				videoPath = p
				break
			}
		}
		if videoPath != "" {
			break
		}
		_ = c
	}
	if videoPath == "" {
		t.Skip("no local video file for ffprobe test")
	}
	locator := video.NewLocator(video.Config{})
	info, err := video.GetVideoInfo(context.Background(), locator, videoPath)
	if err != nil {
		t.Fatal(err)
	}
	if info["video.height"] == nil || info["video.width"] == nil {
		t.Fatalf("missing video fields: %v", info)
	}
}
