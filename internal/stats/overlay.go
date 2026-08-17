package stats

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// OverlayFromVideos fills missing image/video keys by sampling finalized MP4s.
func OverlayFromVideos(existing EpisodeStats, files map[string]string, length int, features map[string]FeatureDesc, opts Options, extra EpisodeInput) (EpisodeStats, error) {
	need := make(map[string]string, len(files))
	for key, path := range files {
		spec, ok := features[key]
		if !ok || (spec.DType != "image" && spec.DType != "video") {
			continue
		}
		if existing[key] != nil {
			continue
		}
		if path == "" {
			continue
		}
		need[key] = path
	}
	if len(need) == 0 {
		return existing, nil
	}
	extra.VideoFiles = need
	extra.Length = length
	more, err := computeEpisodeStats(extra, features, opts)
	if err != nil {
		return existing, err
	}
	out := mergeEpisodeStats(existing, more)
	if missing := MissingVisualKeys(out, need); len(missing) > 0 {
		return out, fmt.Errorf("visual stats missing for %s", strings.Join(missing, ", "))
	}
	return out, nil
}

func mergeEpisodeStats(base, overlay EpisodeStats) EpisodeStats {
	if len(base) == 0 {
		return overlay
	}
	if len(overlay) == 0 {
		return base
	}
	out := make(EpisodeStats, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

// MissingVisualKeys returns feature keys in required that have no stats entry.
func MissingVisualKeys(ep EpisodeStats, required map[string]string) []string {
	var missing []string
	for key := range required {
		if ep[key] == nil {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	return missing
}

// CheckStatsFileVisualCoverage reports when stats.json omits declared image/video keys.
func CheckStatsFileVisualCoverage(path string, features map[string]FeatureDesc) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	keys := make(map[string]any, len(parsed))
	for k := range parsed {
		keys[k] = struct{}{}
	}
	if missing := MissingDeclaredVisualKeys(keys, features); len(missing) > 0 {
		return fmt.Errorf("%s missing image/video features: %s", path, strings.Join(missing, ", "))
	}
	return nil
}

// CheckAggregateVisualCoverage reports when aggregated stats omit declared image/video keys.
func CheckAggregateVisualCoverage(agg map[string]map[string]any, features map[string]FeatureDesc) error {
	keys := make(map[string]any, len(agg))
	for k := range agg {
		keys[k] = struct{}{}
	}
	if missing := MissingDeclaredVisualKeys(keys, features); len(missing) > 0 {
		return fmt.Errorf("aggregated stats missing image/video features: %s", strings.Join(missing, ", "))
	}
	return nil
}

// MissingDeclaredVisualKeys returns image/video feature keys absent from stats.
func MissingDeclaredVisualKeys(statsKeys map[string]any, features map[string]FeatureDesc) []string {
	var missing []string
	for key, spec := range features {
		if spec.DType != "image" && spec.DType != "video" {
			continue
		}
		if _, ok := statsKeys[key]; !ok {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	return missing
}

// AsImageStat311 coerces in-memory or JSON-decoded (C,1,1) stats.
func AsImageStat311(v any) (ImageStat311, bool) {
	switch x := v.(type) {
	case ImageStat311:
		if len(x) == 0 {
			return nil, false
		}
		return x, true
	case [][][]float64:
		if len(x) == 0 {
			return nil, false
		}
		return ImageStat311(x), true
	case []any:
		return imageStat311FromAny(x)
	default:
		return nil, false
	}
}

func imageStat311FromAny(x []any) (ImageStat311, bool) {
	if len(x) == 0 {
		return nil, false
	}
	out := make(ImageStat311, len(x))
	for i, ch := range x {
		planes, ok := ch.([]any)
		if !ok {
			return nil, false
		}
		out[i] = make([][]float64, len(planes))
		for j, plane := range planes {
			vals, ok := plane.([]any)
			if !ok {
				return nil, false
			}
			row := make([]float64, len(vals))
			for k, v := range vals {
				row[k] = toFloat64(v)
			}
			out[i][j] = row
		}
	}
	return out, true
}
