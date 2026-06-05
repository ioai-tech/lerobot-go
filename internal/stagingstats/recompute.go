package stagingstats

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ioai-tech/lerobot-go/internal/manifest"
	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/parquetx"
	"github.com/ioai-tech/lerobot-go/internal/stats"
)

// RecomputeEpisodeStats rebuilds episode stats from staging parquet + image paths.
func RecomputeEpisodeStats(ctx context.Context, stagingDir string, ep manifest.Episode, features map[string]meta.FeatureSpec, opts stats.Options) (stats.EpisodeStats, error) {
	return RecomputeEpisodeStatsWithOptions(ctx, stagingDir, ep, features, opts, nil)
}

// RecomputeEpisodeStatsWithOptions rebuilds stats using staging parquet values after
// applying the same global index/task remapping used during merge.
func RecomputeEpisodeStatsWithOptions(ctx context.Context, stagingDir string, ep manifest.Episode, features map[string]meta.FeatureSpec, opts stats.Options, appendOpts *parquetx.AppendEpisodeOptions) (stats.EpisodeStats, error) {
	cols, err := parquetx.ColumnsForStats(ctx, filepath.Join(stagingDir, ep.FramesParquet))
	if err != nil {
		return nil, err
	}
	if err := applyAppendOverrides(cols, ep.Length, appendOpts); err != nil {
		return nil, err
	}
	desc := make(map[string]stats.FeatureDesc, len(features)+len(meta.DefaultFeatures))
	for k, v := range features {
		desc[k] = stats.FeatureDesc{DType: v.DType, Shape: v.Shape}
	}
	for k, v := range meta.DefaultFeatures {
		desc[k] = stats.FeatureDesc{DType: v.DType, Shape: v.Shape}
	}
	return stats.ComputeEpisodeStats(stats.EpisodeInput{
		Columns:    cols,
		FramePaths: collectFramePaths(stagingDir, features),
	}, desc, opts), nil
}

func applyAppendOverrides(cols map[string]any, length int, appendOpts *parquetx.AppendEpisodeOptions) error {
	if appendOpts == nil {
		return nil
	}
	if _, ok := cols["index"]; ok {
		indices := make([]int64, length)
		for i := range indices {
			indices[i] = appendOpts.GlobalFrameIndex + int64(i)
		}
		cols["index"] = indices
	}
	if _, ok := cols["episode_index"]; ok {
		episodeIndices := make([]int64, length)
		for i := range episodeIndices {
			episodeIndices[i] = appendOpts.EpisodeIndex
		}
		cols["episode_index"] = episodeIndices
	}
	if len(appendOpts.EpisodeTasks) == 0 || len(appendOpts.GlobalTaskIndex) == 0 {
		return nil
	}
	localTaskIndices, ok := cols["task_index"].([]int64)
	if !ok {
		return nil
	}
	localToGlobal := make(map[int64]int64, len(appendOpts.EpisodeTasks)*2)
	for localIdx, task := range appendOpts.EpisodeTasks {
		globalIdx, ok := appendOpts.GlobalTaskIndex[task]
		if !ok {
			return fmt.Errorf("task %q missing from global task map", task)
		}
		g := int64(globalIdx)
		localToGlobal[int64(localIdx)] = g
		localToGlobal[g] = g
	}
	taskIndices := make([]int64, len(localTaskIndices))
	for i, local := range localTaskIndices {
		global, ok := localToGlobal[local]
		if !ok {
			return fmt.Errorf("local task_index %d not mapped", local)
		}
		taskIndices[i] = global
	}
	cols["task_index"] = taskIndices
	return nil
}

func collectFramePaths(stagingDir string, features map[string]meta.FeatureSpec) map[string][]string {
	out := map[string][]string{}
	for key, spec := range features {
		if spec.DType != "image" && spec.DType != "video" {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(stagingDir, "images", key, "frame-*.png"))
		if len(matches) == 0 {
			continue
		}
		sort.Slice(matches, func(i, j int) bool {
			return strings.Compare(filepath.Base(matches[i]), filepath.Base(matches[j])) < 0
		})
		out[key] = matches
	}
	return out
}
