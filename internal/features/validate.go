package features

import (
	"fmt"

	"github.com/ioai-tech/lerobot-go/internal/meta"
)

func MergeWithDefaults(features map[string]meta.FeatureSpec) map[string]meta.FeatureSpec {
	out := make(map[string]meta.FeatureSpec, len(features)+len(meta.DefaultFeatures))
	for k, v := range features {
		out[k] = v
	}
	for k, v := range meta.DefaultFeatures {
		out[k] = v
	}
	return out
}

func ValidateMediaConfig(features map[string]meta.FeatureSpec, useVideos bool) error {
	if useVideos {
		return nil
	}
	for key, spec := range features {
		if spec.DType == "video" {
			return fmt.Errorf("feature %q has dtype %q but UseVideos is false", key, spec.DType)
		}
	}
	return nil
}

func ValidateFrameKeys(frame map[string]any, features map[string]meta.FeatureSpec) error {
	expected := make(map[string]struct{}, len(features))
	for k := range features {
		expected[k] = struct{}{}
	}
	for k := range frame {
		if k == "task" {
			continue
		}
		if _, ok := expected[k]; !ok {
			return fmt.Errorf("unexpected frame key %q", k)
		}
	}
	autoInjected := map[string]struct{}{
		"timestamp": {}, "frame_index": {}, "episode_index": {}, "index": {}, "task_index": {},
	}
	for k, spec := range features {
		if spec.DType == "video" || spec.DType == "image" {
			continue
		}
		if _, auto := autoInjected[k]; auto {
			continue
		}
		if _, ok := frame[k]; !ok {
			return fmt.Errorf("missing required feature %q", k)
		}
	}
	return nil
}

func VideoKeys(features map[string]meta.FeatureSpec) []string {
	var keys []string
	for k, spec := range features {
		if spec.DType == "video" {
			keys = append(keys, k)
		}
	}
	return keys
}
