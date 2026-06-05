package formatcheck

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/parquetx"
)

func expectedDataFeatureColumns(info meta.DatasetInfo) []string {
	var cols []string
	for name, spec := range info.Features {
		switch spec.DType {
		case "video":
			continue
		case "image":
			cols = append(cols, name)
		default:
			cols = append(cols, name)
		}
	}
	sort.Strings(cols)
	return cols
}

func videoFeatureKeys(info meta.DatasetInfo) []string {
	var keys []string
	for name, spec := range info.Features {
		if spec.DType == "video" {
			keys = append(keys, name)
		}
	}
	sort.Strings(keys)
	return keys
}

func checkFeatureColumns(tbl arrow.Table, cols []string, label string, check func(string)) {
	schema := tbl.Schema()
	for _, col := range cols {
		if len(schema.FieldIndices(col)) == 0 {
			check(label + ": missing feature column " + col)
		}
	}
}

func cachedValidationTable(ctx context.Context, cache map[string]arrow.Table, path string) (arrow.Table, error) {
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

func checkEpisodeMetaV30(root string, info meta.DatasetInfo, check func(string)) {
	rows, err := parquetx.ReadEpisodesMeta(root)
	if err != nil {
		check("episodes meta: " + err.Error())
		return
	}
	if len(rows) != info.TotalEpisodes {
		check(fmt.Sprintf("episodes meta rows %d != total_episodes %d", len(rows), info.TotalEpisodes))
	}
	var frameSum int64
	var expectFrom int64
	seen := map[int64]bool{}
	for _, ep := range rows {
		if seen[ep.EpisodeIndex] {
			check(fmt.Sprintf("duplicate episode_index %d in episodes meta", ep.EpisodeIndex))
		}
		seen[ep.EpisodeIndex] = true
		if ep.DatasetFromIndex != expectFrom {
			check(fmt.Sprintf("episode %d: dataset_from_index %d want %d", ep.EpisodeIndex, ep.DatasetFromIndex, expectFrom))
		}
		if ep.DatasetToIndex != ep.DatasetFromIndex+ep.Length {
			check(fmt.Sprintf("episode %d: dataset_to_index %d != from+length %d", ep.EpisodeIndex, ep.DatasetToIndex, ep.DatasetFromIndex+ep.Length))
		}
		frameSum += ep.Length
		expectFrom = ep.DatasetToIndex
	}
	if int(frameSum) != info.TotalFrames {
		check(fmt.Sprintf("episodes meta length sum %d != total_frames %d", frameSum, info.TotalFrames))
	}
}

func checkTasksParquetV30(ctx context.Context, root string, info meta.DatasetInfo, check func(string)) {
	path := filepath.Join(root, meta.DefaultTasksPath)
	n, err := parquetx.TableNumRows(path)
	if err != nil {
		check("tasks.parquet: " + err.Error())
		return
	}
	if int(n) != info.TotalTasks {
		check(fmt.Sprintf("tasks.parquet rows %d != total_tasks %d", n, info.TotalTasks))
	}
	taskMap, err := parquetx.ReadTasksParquet(ctx, path)
	if err != nil {
		check("tasks.parquet: " + err.Error())
		return
	}
	for i := 0; i < info.TotalTasks; i++ {
		found := false
		for _, idx := range taskMap {
			if idx == i {
				found = true
				break
			}
		}
		if !found {
			check(fmt.Sprintf("tasks.parquet: missing task_index %d", i))
			break
		}
	}
}

func checkVideosV30(root string, info meta.DatasetInfo, check, warn func(string)) {
	keys := videoFeatureKeys(info)
	if len(keys) == 0 || info.VideoPath == nil {
		return
	}
	rows, err := parquetx.ReadEpisodesMeta(root)
	if err != nil {
		return
	}
	for _, ep := range rows {
		for _, key := range keys {
			chunkKey := "videos/" + key + "/chunk_index"
			fileKey := "videos/" + key + "/file_index"
			chunk := firstFloat(ep.VideoFields[chunkKey])
			file := firstFloat(ep.VideoFields[fileKey])
			if chunk < 0 || file < 0 {
				warn(fmt.Sprintf("episode %d: missing video meta for %s", ep.EpisodeIndex, key))
				continue
			}
			rel := meta.VideoPath(key, int(chunk), int(file))
			if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
				check(fmt.Sprintf("episode %d: missing video %s", ep.EpisodeIndex, rel))
			}
		}
	}
}

func checkVideosV21(root string, info meta.DatasetInfo, check func(string)) {
	keys := videoFeatureKeys(info)
	if len(keys) == 0 || info.VideoPath == nil {
		return
	}
	for ep := 0; ep < info.TotalEpisodes; ep++ {
		for _, key := range keys {
			rel := meta.V21VideoPathFromInfo(info, key, ep)
			if rel == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
				// Some datasets use camera folder basename only.
				alt := meta.V21VideoPathFromInfo(info, filepath.Base(key), ep)
				if _, err2 := os.Stat(filepath.Join(root, alt)); err2 != nil {
					check(fmt.Sprintf("episode %d: missing video %s", ep, rel))
				}
			}
		}
	}
}

func checkTimestamps(tbl arrow.Table, fps int, label string, check, warn func(string)) {
	if fps <= 0 {
		return
	}
	ts, err := parquetx.ExtractFloat64Column(tbl, "timestamp")
	if err != nil {
		warn(label + ": cannot read timestamp column")
		return
	}
	episodeIndices, err := parquetx.ExtractInt64Column(tbl, "episode_index")
	if err != nil {
		episodeIndices = nil
	}
	step := 1.0 / float64(fps)
	tol := 1e-3
	for i := 1; i < len(ts); i++ {
		if len(episodeIndices) == len(ts) && episodeIndices[i] != episodeIndices[i-1] {
			continue
		}
		delta := ts[i] - ts[i-1]
		if math.Abs(delta-step) > tol && math.Abs(delta) > tol {
			warn(fmt.Sprintf("%s: timestamp delta row %d = %.6f, want ~%.6f", label, i, delta, step))
			break
		}
	}
}

func firstFloat(v any) float64 {
	switch x := v.(type) {
	case []float64:
		if len(x) > 0 {
			return x[0]
		}
	case []int64:
		if len(x) > 0 {
			return float64(x[0])
		}
	case []any:
		if len(x) > 0 {
			switch n := x[0].(type) {
			case float64:
				return n
			case int64:
				return float64(n)
			}
		}
	case float64:
		return x
	case int64:
		return float64(x)
	}
	return -1
}

func v21EpisodeDataPaths(root string, info meta.DatasetInfo) []string {
	var paths []string
	for ep := 0; ep < info.TotalEpisodes; ep++ {
		paths = append(paths, meta.V21DataPathFromInfo(info, ep))
	}
	// Fallback: official convert script globs data/*/*.parquet when templates fail.
	if len(paths) == 1 {
		if _, err := os.Stat(filepath.Join(root, paths[0])); err != nil {
			glob, _ := filepath.Glob(filepath.Join(root, meta.DataDir, "*", "*.parquet"))
			sort.Strings(glob)
			if len(glob) > 0 {
				rel := make([]string, len(glob))
				for i, p := range glob {
					rel[i], _ = filepath.Rel(root, p)
				}
				return rel
			}
		}
	}
	return paths
}

func hasDataDir(root string) bool {
	st, err := os.Stat(filepath.Join(root, meta.DataDir))
	return err == nil && st.IsDir()
}

func warnMissingStats(strict bool, warn func(string)) {
	if strict {
		warn("meta/stats.json missing (LeRobot load_stats returns None)")
	}
}
