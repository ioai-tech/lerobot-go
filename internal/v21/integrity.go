package v21

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/stats"
)

func requiredVideoKeys(features map[string]meta.FeatureSpec) []string {
	var keys []string
	for k, f := range features {
		if f.DType == "video" {
			keys = append(keys, k)
		}
	}
	return keys
}

// ValidateOutputIntegrity checks merged v2.1 dataset has per-episode parquet and video files.
func ValidateOutputIntegrity(outputRoot string, info meta.DatasetInfo, features map[string]meta.FeatureSpec) error {
	if info.TotalEpisodes <= 0 {
		matches, err := filepath.Glob(filepath.Join(outputRoot, meta.DataDir, "*", "episode_*.parquet"))
		if err != nil {
			return err
		}
		if len(matches) == 0 {
			matches, err = filepath.Glob(filepath.Join(outputRoot, meta.DataDir, "*", "*.parquet"))
			if err != nil {
				return err
			}
		}
		if len(matches) == 0 {
			return fmt.Errorf("no v2.1 data parquet files under %s", outputRoot)
		}
		for _, p := range matches {
			if err := nonEmptyFile(p); err != nil {
				return err
			}
		}
	} else {
		for ep := 0; ep < info.TotalEpisodes; ep++ {
			p := filepath.Join(outputRoot, meta.V21DataPathFromInfo(info, ep))
			if err := nonEmptyFile(p); err != nil {
				return fmt.Errorf("episode %d: %w", ep, err)
			}
		}
	}
	if err := stats.CheckStatsFileVisualCoverage(filepath.Join(outputRoot, meta.StatsPath), featureDescs(features)); err != nil {
		return err
	}
	if !hasVideoFeatures(features) {
		return nil
	}
	for _, vk := range requiredVideoKeys(features) {
		found := 0
		for ep := 0; ep < max(1, info.TotalEpisodes); ep++ {
			p := filepath.Join(outputRoot, meta.V21VideoPathFromInfo(info, vk, ep))
			if err := nonEmptyFile(p); err == nil {
				found++
				continue
			}
			alt := filepath.Join(outputRoot, meta.V21VideoPathFromInfo(info, filepath.Base(vk), ep))
			if err := nonEmptyFile(alt); err == nil {
				found++
			}
		}
		if found == 0 {
			glob, _ := filepath.Glob(filepath.Join(outputRoot, "videos", "*", vk, "episode_*.mp4"))
			if len(glob) == 0 {
				glob, _ = filepath.Glob(filepath.Join(outputRoot, "videos", "*", filepath.Base(vk), "episode_*.mp4"))
			}
			if len(glob) == 0 {
				return fmt.Errorf("no merged video files for feature %q", vk)
			}
			for _, f := range glob {
				if err := nonEmptyFile(f); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func nonEmptyFile(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("file missing %s: %w", path, err)
	}
	if fi.Size() == 0 {
		return fmt.Errorf("file empty: %s", path)
	}
	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
