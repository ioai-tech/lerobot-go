package convert

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ioai-tech/lerobot-go/internal/jsonutil"
	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/parquetx"
	"github.com/ioai-tech/lerobot-go/internal/stats"
	"github.com/ioai-tech/lerobot-go/internal/video"
)

func V21ToV30(ctx context.Context, cfg Config) error {
	info, err := meta.LoadInfo(cfg.Input)
	if err != nil {
		return err
	}
	if info.CodebaseVersion != meta.CodebaseV21 {
		return fmt.Errorf("input version is %s, want v2.1", info.CodebaseVersion)
	}

	episodes, err := jsonutil.ReadJSONL(filepath.Join(cfg.Input, meta.LegacyEpisodesPath))
	if err != nil {
		return err
	}
	epStatsRows, err := jsonutil.ReadJSONL(filepath.Join(cfg.Input, meta.LegacyEpisodesStatsPath))
	if err != nil {
		return err
	}
	statsByEp := map[int]stats.EpisodeStats{}
	for _, row := range epStatsRows {
		ep, _ := row["episode_index"].(float64)
		raw, _ := row["stats"].(map[string]any)
		statsByEp[int(ep)] = toEpisodeStats(raw)
	}

	taskMap, err := loadV21Tasks(cfg.Input)
	if err != nil {
		return err
	}

	outInfo := info
	outInfo.CodebaseVersion = meta.CodebaseV30
	outInfo.TotalChunks = nil
	outInfo.TotalVideos = nil
	outInfo.DataFilesSizeInMB = cfg.DataFileSizeMB
	outInfo.VideoFilesSizeInMB = cfg.VideoFileSizeMB
	outInfo.ChunksSize = cfg.ChunksSize
	outInfo.DataPath = "data/chunk-{chunk_index:03d}/file-{file_index:03d}.parquet"
	if info.VideoPath != nil {
		p := "videos/{video_key}/chunk-{chunk_index:03d}/file-{file_index:03d}.mp4"
		outInfo.VideoPath = &p
	}

	epPaths, err := sortedV21ParquetPaths(cfg.Input, info.TotalEpisodes)
	if err != nil {
		return err
	}

	chunk, file := 0, 0
	globalFrame := int64(0)
	var epMetaInputs []parquetx.EpisodeMetaInput

	for i, epPath := range epPaths {
		epLegacy := episodes[i]
		tasks := stringList(epLegacy["tasks"])
		epIdx := int(epLegacy["episode_index"].(float64))
		length := int(epLegacy["length"].(float64))

		if err := maybeRotateData(cfg.Output, cfg, &chunk, &file, epPath); err != nil {
			return err
		}
		dst := filepath.Join(cfg.Output, meta.DataPath(chunk, file))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := parquetx.AppendEpisodeParquet(ctx, dst, epPath, parquetx.AppendEpisodeOptions{
			GlobalFrameIndex: globalFrame,
			EpisodeIndex:     int64(epIdx),
			EpisodeTasks:     tasks,
			GlobalTaskIndex:  taskMap,
		}); err != nil {
			return err
		}

		fromIndex := globalFrame
		globalFrame += int64(length)
		epMetaInputs = append(epMetaInputs, parquetx.EpisodeMetaInput{
			EpisodeIndex: epIdx,
			Tasks:        tasks,
			Length:       length,
			Fields: map[string]any{
				"data/chunk_index":          chunk,
				"data/file_index":           file,
				"dataset_from_index":        fromIndex,
				"dataset_to_index":          globalFrame,
				"meta/episodes/chunk_index": 0,
				"meta/episodes/file_index":  0,
			},
			Stats: statsByEp[epIdx],
		})

		dstSize, _ := meta.ParquetUncompressedSizeMB(dst)
		avg := dstSize / float64(max(1, length))
		if dstSize+avg*float64(length) >= float64(cfg.DataFileSizeMB) {
			chunk, file = meta.UpdateChunkFileIndices(chunk, file, cfg.ChunksSize)
		}
	}

	if err := convertV21Videos(ctx, cfg, epMetaInputs); err != nil {
		return err
	}
	if err := parquetx.WriteTasksParquet(cfg.Output, taskMap); err != nil {
		return err
	}
	if err := parquetx.WriteEpisodesParquet(cfg.Output, epMetaInputs, cfg.DataFileSizeMB); err != nil {
		return err
	}
	if err := copyStats(cfg.Input, cfg.Output); err != nil {
		return err
	}
	return meta.WriteInfo(cfg.Output, outInfo)
}

func maybeRotateData(output string, cfg Config, chunk, file *int, src string) error {
	dst := filepath.Join(output, meta.DataPath(*chunk, *file))
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		return nil
	}
	dstSize, err := meta.ParquetUncompressedSizeMB(dst)
	if err != nil {
		return err
	}
	srcSize, err := meta.ParquetUncompressedSizeMB(src)
	if err != nil {
		return err
	}
	if dstSize+srcSize >= float64(cfg.DataFileSizeMB) {
		*chunk, *file = meta.UpdateChunkFileIndices(*chunk, *file, cfg.ChunksSize)
	}
	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func loadV21Tasks(root string) (map[string]int, error) {
	rows, err := jsonutil.ReadJSONL(filepath.Join(root, meta.LegacyTasksPath))
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		task, _ := row["task"].(string)
		idx, _ := row["task_index"].(float64)
		out[task] = int(idx)
	}
	return out, nil
}

func sortedV21ParquetPaths(root string, n int) ([]string, error) {
	var paths []string
	info, err := meta.LoadInfo(root)
	if err != nil {
		return nil, err
	}
	for ep := 0; ep < n; ep++ {
		rel := meta.V21DataPathFromInfo(info, ep)
		p := filepath.Join(root, rel)
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("missing %s", rel)
		}
		paths = append(paths, p)
	}
	return paths, nil
}

func stringList(v any) []string {
	switch x := v.(type) {
	case []any:
		out := make([]string, len(x))
		for i, e := range x {
			out[i], _ = e.(string)
		}
		return out
	case []string:
		return x
	default:
		return nil
	}
}

func toEpisodeStats(raw map[string]any) stats.EpisodeStats {
	if raw == nil {
		return nil
	}
	out := make(stats.EpisodeStats, len(raw))
	for k, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out[k] = stats.FeatureStats(m)
		}
	}
	return out
}

func copyStats(input, output string) error {
	data, err := os.ReadFile(filepath.Join(input, meta.StatsPath))
	if err != nil {
		return err
	}
	var s map[string]map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	return meta.WriteStats(output, s)
}

func convertV21Videos(ctx context.Context, cfg Config, epMeta []parquetx.EpisodeMetaInput) error {
	info, _ := meta.LoadInfo(cfg.Input)
	var videoKeys []string
	for k, f := range info.Features {
		if f.DType == "video" {
			videoKeys = append(videoKeys, k)
		}
	}
	if len(videoKeys) == 0 {
		return nil
	}
	sort.Strings(videoKeys)
	for _, key := range videoKeys {
		if err := convertV21Camera(ctx, cfg, key, epMeta); err != nil {
			return err
		}
	}
	return nil
}

func v21VideoPath(root, videoKey string, ep int) string {
	info, err := meta.LoadInfo(root)
	if err != nil {
		return ""
	}
	p1 := filepath.Join(root, meta.V21VideoPathFromInfo(info, videoKey, ep))
	if _, err := os.Stat(p1); err == nil {
		return p1
	}
	return filepath.Join(root, meta.V21VideoPathFromInfo(info, filepath.Base(videoKey), ep))
}

func convertV21Camera(ctx context.Context, cfg Config, videoKey string, epMeta []parquetx.EpisodeMetaInput) error {
	chunk, file := 0, 0
	var sizeMB float64
	var paths []string
	var dur float64
	startEp := 0

	flush := func(upTo int) error {
		if len(paths) == 0 {
			return nil
		}
		out := filepath.Join(cfg.Output, meta.VideoPath(videoKey, chunk, file))
		if err := video.SafeConcat(ctx, cfg.Locator, paths, out, true); err != nil {
			return err
		}
		for ep := startEp; ep < upTo; ep++ {
			epMeta[ep].Fields["videos/"+videoKey+"/chunk_index"] = chunk
			epMeta[ep].Fields["videos/"+videoKey+"/file_index"] = file
		}
		paths = nil
		sizeMB = 0
		startEp = upTo
		return nil
	}

	for ep := 0; ep < len(epMeta); ep++ {
		src := v21VideoPath(cfg.Input, videoKey, ep)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		epSize, _ := video.FileSizeMB(src)
		epDur, _ := video.DurationSeconds(ctx, cfg.Locator, src)
		if sizeMB+epSize >= float64(cfg.VideoFileSizeMB) && len(paths) > 0 {
			if err := flush(ep); err != nil {
				return err
			}
			chunk, file = meta.UpdateChunkFileIndices(chunk, file, cfg.ChunksSize)
			dur = 0
		}
		fromTS := dur
		dur += epDur
		epMeta[ep].Fields["videos/"+videoKey+"/from_timestamp"] = fromTS
		epMeta[ep].Fields["videos/"+videoKey+"/to_timestamp"] = dur
		epMeta[ep].Fields["videos/"+videoKey+"/chunk_index"] = chunk
		epMeta[ep].Fields["videos/"+videoKey+"/file_index"] = file
		paths = append(paths, src)
		sizeMB += epSize
	}
	return flush(len(epMeta))
}
