package v30

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ioai-tech/lerobot-go/internal/meta"
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

// ValidateOutputIntegrity checks merged dataset has data parquet and video files.
func ValidateOutputIntegrity(outputRoot string, features map[string]meta.FeatureSpec) error {
	matches, err := filepath.Glob(filepath.Join(outputRoot, "data", "chunk-*", "file-*.parquet"))
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return fmt.Errorf("no data parquet files under %s", outputRoot)
	}
	for _, p := range matches {
		fi, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("data parquet missing: %w", err)
		}
		if fi.Size() == 0 {
			return fmt.Errorf("data parquet empty: %s", p)
		}
	}
	if !hasVideoFeatures(features) {
		return nil
	}
	for _, vk := range requiredVideoKeys(features) {
		pattern := filepath.Join(outputRoot, "videos", vk, "chunk-*", "file-*.mp4")
		vm, err := filepath.Glob(pattern)
		if err != nil {
			return err
		}
		if len(vm) == 0 {
			return fmt.Errorf("no merged video files for feature %q", vk)
		}
		for _, f := range vm {
			fi, err := os.Stat(f)
			if err != nil {
				return fmt.Errorf("video file missing %s: %w", f, err)
			}
			if fi.Size() == 0 {
				return fmt.Errorf("video file empty: %s", f)
			}
		}
	}
	return nil
}
