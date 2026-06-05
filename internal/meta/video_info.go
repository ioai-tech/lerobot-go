package meta

import (
	"context"
	"os"
	"path/filepath"

	"github.com/ioai-tech/lerobot-go/internal/video"
)

// UpdateVideoFeaturesInfo fills features[].info from the first on-disk video file,
// matching dataset_metadata.update_video_info (lerobot v0.5.1).
func UpdateVideoFeaturesInfo(ctx context.Context, info *DatasetInfo, root string, locator video.Locator) error {
	if info.VideoPath == nil {
		return nil
	}
	for key, spec := range info.Features {
		if spec.DType != "video" {
			continue
		}
		if len(spec.Info) > 0 {
			continue
		}
		videoPath := filepath.Join(root, V21VideoPathFromInfo(*info, key, 0))
		if _, err := os.Stat(videoPath); err != nil {
			if info.CodebaseVersion == CodebaseV30 {
				videoPath = filepath.Join(root, VideoPath(key, 0, 0))
				if _, err2 := os.Stat(videoPath); err2 != nil {
					continue
				}
			} else {
				continue
			}
		}
		vinfo, err := video.GetVideoInfo(ctx, locator, videoPath)
		if err != nil {
			return err
		}
		if len(vinfo) == 0 {
			continue
		}
		spec.Info = vinfo
		info.Features[key] = spec
	}
	return nil
}
