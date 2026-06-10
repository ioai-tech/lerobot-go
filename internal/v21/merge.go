package v21

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ioai-tech/lerobot-go/internal/manifest"
	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/parquetx"
	"github.com/ioai-tech/lerobot-go/internal/stagingstats"
	"github.com/ioai-tech/lerobot-go/internal/stats"
	"github.com/ioai-tech/lerobot-go/internal/video"
)

type MergeConfig struct {
	StagingRoot string
	OutputRoot  string
	RepoID      string
	RobotType   string
	FPS         int
	Features    map[string]meta.FeatureSpec
	Locator     video.Locator
	Stats       stats.Options
}

func Merge(ctx context.Context, cfg MergeConfig) error {
	dirs, err := manifest.ListStagingEpisodes(cfg.StagingRoot)
	if err != nil {
		return err
	}
	if len(dirs) == 0 {
		return fmt.Errorf("no staging episodes in %s", cfg.StagingRoot)
	}
	useVideos := hasVideoFeatures(cfg.Features)
	info := meta.NewDatasetInfo(meta.CodebaseV21, cfg.FPS, cfg.Features, useVideos, cfg.RobotType)
	taskMap := map[string]int{}
	var allStats []stats.EpisodeStats
	globalIndex := int64(0)
	videoKeys := videoFeatureKeys(cfg.Features)

	for _, dir := range dirs {
		ep, err := manifest.Read(dir)
		if err != nil {
			return err
		}
		for _, t := range ep.Tasks {
			if _, ok := taskMap[t]; !ok {
				taskMap[t] = len(taskMap)
			}
		}
		// copy parquet
		srcPQ := filepath.Join(dir, ep.FramesParquet)
		dstPQ := filepath.Join(cfg.OutputRoot, meta.V21DataPathFromInfo(info, ep.EpisodeIndex))
		if err := copyReplaceIndex(srcPQ, dstPQ, globalIndex, ep.EpisodeIndex, taskMap, ep.Tasks); err != nil {
			return err
		}
		for videoKey, rel := range ep.Videos {
			src := manifest.StagingMediaPath(dir, rel)
			dst := filepath.Join(cfg.OutputRoot, meta.V21VideoPathFromInfo(info, videoKey, ep.EpisodeIndex))
			if err := copyFile(src, dst); err != nil {
				return err
			}
		}
		if err := appendJSONL(cfg.OutputRoot, meta.LegacyEpisodesPath, map[string]any{
			"episode_index": ep.EpisodeIndex,
			"tasks":         ep.Tasks,
			"length":        ep.Length,
		}); err != nil {
			return err
		}
		epStats := ep.Stats
		if recomputed, err := stagingstats.RecomputeEpisodeStatsWithOptions(ctx, dir, ep, cfg.Features, cfg.Stats, &parquetx.AppendEpisodeOptions{
			GlobalFrameIndex: globalIndex,
			EpisodeIndex:     int64(ep.EpisodeIndex),
			EpisodeTasks:     ep.Tasks,
			GlobalTaskIndex:  taskMap,
		}); err == nil && len(recomputed) > 0 {
			epStats = mergeEpisodeStats(epStats, recomputed)
		}
		if err := appendJSONL(cfg.OutputRoot, meta.LegacyEpisodesStatsPath, map[string]any{
			"episode_index": ep.EpisodeIndex,
			"stats":         convertEpisodeStats(epStats),
		}); err != nil {
			return err
		}
		allStats = append(allStats, epStats)
		globalIndex += int64(ep.Length)
		info.TotalEpisodes++
		info.TotalFrames += ep.Length
		if info.TotalVideos != nil {
			*info.TotalVideos += len(videoKeys)
		}
		chunk := meta.EpisodeChunk(ep.EpisodeIndex, info.ChunksSize)
		if info.TotalChunks != nil && chunk >= *info.TotalChunks {
			*info.TotalChunks = chunk + 1
		}
	}
	info.TotalTasks = len(taskMap)
	if err := writeTasksJSONL(cfg.OutputRoot, taskMap); err != nil {
		return err
	}
	agg := stats.AggregateStats(allStats)
	if err := meta.WriteStats(cfg.OutputRoot, stats.ToJSONSerializable(agg)); err != nil {
		return err
	}
	if cfg.Locator == nil {
		cfg.Locator = video.NewLocator(video.Config{})
	}
	if err := meta.UpdateVideoFeaturesInfo(ctx, &info, cfg.OutputRoot, cfg.Locator); err != nil {
		return err
	}
	if err := meta.WriteInfo(cfg.OutputRoot, info); err != nil {
		return err
	}
	if err := ValidateOutputIntegrity(cfg.OutputRoot, info, cfg.Features); err != nil {
		return fmt.Errorf("merged output integrity: %w", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func appendJSONL(root, rel string, row map[string]any) error {
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	data, err := json.Marshal(row)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

func writeTasksJSONL(root string, tasks map[string]int) error {
	path := filepath.Join(root, meta.LegacyTasksPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	inv := make([]string, len(tasks))
	for t, i := range tasks {
		inv[i] = t
	}
	for i, task := range inv {
		row := map[string]any{"task_index": i, "task": task}
		data, err := json.Marshal(row)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func copyReplaceIndex(src, dst string, globalIndex int64, episodeIndex int, taskMap map[string]int, episodeTasks []string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tbl, err := parquetx.RewriteEpisodeParquet(context.Background(), src, parquetx.AppendEpisodeOptions{
		GlobalFrameIndex: globalIndex,
		EpisodeIndex:     int64(episodeIndex),
		EpisodeTasks:     episodeTasks,
		GlobalTaskIndex:  taskMap,
	}, nil)
	if err != nil {
		return err
	}
	defer tbl.Release()
	return parquetx.WriteTable(dst, tbl, nil)
}

func hasVideoFeatures(features map[string]meta.FeatureSpec) bool {
	for _, f := range features {
		if f.DType == "video" {
			return true
		}
	}
	return false
}

func videoFeatureKeys(features map[string]meta.FeatureSpec) []string {
	var keys []string
	for k, f := range features {
		if f.DType == "video" {
			keys = append(keys, k)
		}
	}
	return keys
}

func convertEpisodeStats(ep stats.EpisodeStats) map[string]any {
	out := map[string]any{}
	for k, v := range ep {
		out[k] = v
	}
	return out
}

func mergeEpisodeStats(base, overlay stats.EpisodeStats) stats.EpisodeStats {
	if len(base) == 0 {
		return overlay
	}
	if len(overlay) == 0 {
		return base
	}
	out := make(stats.EpisodeStats, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}
