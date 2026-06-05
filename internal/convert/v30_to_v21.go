package convert

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/ioai-tech/lerobot-go/internal/jsonutil"
	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/parquetx"
	"github.com/ioai-tech/lerobot-go/internal/video"
)

func V30ToV21(ctx context.Context, cfg Config) error {
	info, err := meta.LoadInfo(cfg.Input)
	if err != nil {
		return err
	}
	if info.CodebaseVersion != meta.CodebaseV30 {
		return fmt.Errorf("input version is %s, want v3.0", info.CodebaseVersion)
	}
	if err := os.MkdirAll(filepath.Join(cfg.Output, "meta"), 0o755); err != nil {
		return err
	}

	epRows, err := parquetx.ReadEpisodesMeta(cfg.Input)
	if err != nil {
		return err
	}
	taskMap, err := parquetx.ReadTasksParquet(ctx, filepath.Join(cfg.Input, meta.DefaultTasksPath))
	if err != nil {
		return err
	}

	tableCache := map[string]arrow.Table{}
	defer func() {
		for _, tbl := range tableCache {
			tbl.Release()
		}
	}()

	outInfo := info
	outInfo.CodebaseVersion = meta.CodebaseV21
	z := 0
	outInfo.TotalChunks = &z
	outInfo.TotalVideos = &z
	outInfo.DataPath = meta.V21DefaultDataPathTemplate
	if info.VideoPath != nil {
		p := meta.V21DefaultVideoPathTemplate
		outInfo.VideoPath = &p
	}

	var epJSONL []map[string]any
	var epStatsJSONL []map[string]any
	taskInv := invertTaskMap(taskMap)

	for _, ep := range epRows {
		srcPath := filepath.Join(cfg.Input, meta.DataPath(int(ep.DataChunkIndex), int(ep.DataFileIndex)))
		srcTable, err := cachedTable(ctx, tableCache, srcPath)
		if err != nil {
			return fmt.Errorf("episode %d: %w", ep.EpisodeIndex, err)
		}
		slice, err := parquetx.SliceTableByIndexRange(srcTable, ep.DatasetFromIndex, ep.DatasetToIndex)
		if err != nil {
			return fmt.Errorf("episode %d: %w", ep.EpisodeIndex, err)
		}
		indices := make([]int64, slice.NumRows())
		epIndices := make([]int64, slice.NumRows())
		for i := range indices {
			indices[i] = ep.DatasetFromIndex + int64(i)
			epIndices[i] = ep.EpisodeIndex
		}
		step, err := parquetx.ReplaceInt64Column(slice, "index", indices, nil)
		slice.Release()
		if err != nil {
			return err
		}
		final, err := parquetx.ReplaceInt64Column(step, "episode_index", epIndices, nil)
		step.Release()
		if err != nil {
			return err
		}
		dst := filepath.Join(cfg.Output, meta.V21DataPathFromInfo(outInfo, int(ep.EpisodeIndex)))
		if err := parquetx.WriteTable(dst, final, nil); err != nil {
			final.Release()
			return err
		}
		final.Release()

		epJSONL = append(epJSONL, map[string]any{
			"episode_index": ep.EpisodeIndex,
			"tasks":         ep.Tasks,
			"length":        ep.Length,
		})
		statsObj := parquetx.UnflattenEpisodeStats(ep.StatsFields)
		if len(statsObj) == 0 {
			statsObj = map[string]any{}
		}
		epStatsJSONL = append(epStatsJSONL, map[string]any{
			"episode_index": ep.EpisodeIndex,
			"stats":         statsObj,
		})
	}

	if err := convertV30Videos(ctx, cfg, epRows); err != nil {
		return err
	}
	if err := jsonutil.WriteJSONL(filepath.Join(cfg.Output, meta.LegacyEpisodesPath), epJSONL); err != nil {
		return err
	}
	if err := jsonutil.WriteJSONL(filepath.Join(cfg.Output, meta.LegacyEpisodesStatsPath), epStatsJSONL); err != nil {
		return err
	}
	if err := writeV21Tasks(cfg.Output, taskInv); err != nil {
		return err
	}
	if err := copyStats(cfg.Input, cfg.Output); err != nil {
		return err
	}
	outInfo.TotalEpisodes = len(epRows)
	var totalFrames int
	for _, ep := range epRows {
		totalFrames += int(ep.Length)
	}
	outInfo.TotalFrames = totalFrames
	return meta.WriteInfo(cfg.Output, outInfo)
}

func cachedTable(ctx context.Context, cache map[string]arrow.Table, path string) (arrow.Table, error) {
	if tbl, ok := cache[path]; ok {
		return tbl, nil
	}
	tbl, err := parquetx.ReadTable(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	cache[path] = tbl
	return tbl, nil
}

func invertTaskMap(m map[string]int) []string {
	inv := make([]string, len(m))
	for task, idx := range m {
		if idx >= 0 && idx < len(inv) {
			inv[idx] = task
		}
	}
	return inv
}

func writeV21Tasks(root string, tasks []string) error {
	if err := os.MkdirAll(filepath.Join(root, "meta"), 0o755); err != nil {
		return err
	}
	var rows []map[string]any
	for i, task := range tasks {
		rows = append(rows, map[string]any{"task_index": i, "task": task})
	}
	return jsonutil.WriteJSONL(filepath.Join(root, meta.LegacyTasksPath), rows)
}

func convertV30Videos(ctx context.Context, cfg Config, epRows []parquetx.EpisodeMetaRow) error {
	info, _ := meta.LoadInfo(cfg.Input)
	v21Info := info
	v21Info.CodebaseVersion = meta.CodebaseV21
	v21Info.DataPath = meta.V21DefaultDataPathTemplate
	if info.VideoPath != nil {
		p := meta.V21DefaultVideoPathTemplate
		v21Info.VideoPath = &p
	}
	var videoKeys []string
	for k, f := range info.Features {
		if f.DType == "video" {
			videoKeys = append(videoKeys, k)
		}
	}
	if len(videoKeys) == 0 {
		return nil
	}
	for _, key := range videoKeys {
		for _, ep := range epRows {
			chunkKey := fmt.Sprintf("videos/%s/chunk_index", key)
			fileKey := fmt.Sprintf("videos/%s/file_index", key)
			fromKey := fmt.Sprintf("videos/%s/from_timestamp", key)
			toKey := fmt.Sprintf("videos/%s/to_timestamp", key)
			chunk := float64Slice(ep.VideoFields[chunkKey])
			file := float64Slice(ep.VideoFields[fileKey])
			from := float64Slice(ep.VideoFields[fromKey])
			to := float64Slice(ep.VideoFields[toKey])
			if len(chunk) == 0 || len(file) == 0 {
				continue
			}
			src := filepath.Join(cfg.Input, meta.VideoPath(key, int(chunk[0]), int(file[0])))
			dst := filepath.Join(cfg.Output, meta.V21VideoPathFromInfo(v21Info, key, int(ep.EpisodeIndex)))
			if err := video.ExtractSegment(ctx, cfg.Locator, src, dst, from[0], to[0], true); err != nil {
				dst2 := filepath.Join(cfg.Output, meta.V21VideoPathFromInfo(v21Info, filepath.Base(key), int(ep.EpisodeIndex)))
				if err2 := video.ExtractSegment(ctx, cfg.Locator, src, dst2, from[0], to[0], true); err2 != nil {
					return err
				}
			}
		}
	}
	return nil
}

func float64Slice(v any) []float64 {
	switch x := v.(type) {
	case []float64:
		return x
	case []any:
		out := make([]float64, len(x))
		for i, e := range x {
			out[i], _ = e.(float64)
		}
		return out
	default:
		return nil
	}
}
