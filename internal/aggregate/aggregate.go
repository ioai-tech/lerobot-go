package aggregate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/ioai-tech/lerobot-go/internal/convert"
	"github.com/ioai-tech/lerobot-go/internal/jsonutil"
	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/parquetx"
	"github.com/ioai-tech/lerobot-go/internal/stats"
	"github.com/ioai-tech/lerobot-go/internal/video"
	"github.com/ioai-tech/lerobot-go/lerobot"
)

type Config struct {
	Inputs                                      []string
	Output                                      string
	To                                          lerobot.Version
	DataFileSizeMB, VideoFileSizeMB, ChunksSize int
	Locator                                     video.Locator
	Stats                                       stats.Options
}

type dataBatch struct {
	chunk   int
	file    int
	sizeMB  float64
	tables  []arrow.Table
	rowNums []int64
}

type videoBatch struct {
	key          string
	chunk        int
	file         int
	segmentPaths []string
}

type videoChunkState struct {
	key          string
	chunk        int
	file         int
	sizeMB       float64
	duration     float64
	segmentPaths []string
	batches      []videoBatch
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Locator == nil {
		cfg.Locator = video.NewLocator(video.Config{})
	}
	if cfg.DataFileSizeMB <= 0 {
		cfg.DataFileSizeMB = meta.DefaultDataFileSizeInMB
	}
	if cfg.VideoFileSizeMB <= 0 {
		cfg.VideoFileSizeMB = meta.DefaultVideoFileSizeInMB
	}
	if cfg.ChunksSize <= 0 {
		cfg.ChunksSize = meta.DefaultChunkSize
	}
	if cfg.To == lerobot.V21 {
		tmp, err := os.MkdirTemp("", "lerobot-go-merge-*")
		if err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(tmp) }()
		v30cfg := cfg
		v30cfg.Output = tmp
		v30cfg.To = lerobot.V30
		if err := mergeV30(ctx, v30cfg); err != nil {
			return err
		}
		return convert.Run(ctx, convert.Config{
			Input: tmp, Output: cfg.Output,
			From: lerobot.V30, To: lerobot.V21,
			DataFileSizeMB: cfg.DataFileSizeMB, VideoFileSizeMB: cfg.VideoFileSizeMB,
			ChunksSize: cfg.ChunksSize, Locator: cfg.Locator,
		})
	}
	return mergeV30(ctx, cfg)
}

func mergeV30(ctx context.Context, cfg Config) error {
	infos := make([]meta.DatasetInfo, len(cfg.Inputs))
	for i, root := range cfg.Inputs {
		info, err := meta.LoadInfo(root)
		if err != nil {
			return err
		}
		infos[i] = info
	}
	if err := validateCompatible(infos); err != nil {
		return err
	}

	base := infos[0]
	useVideos := hasVideo(base.Features)
	robot := ""
	if base.RobotType != nil {
		robot = *base.RobotType
	}
	outInfo := meta.NewDatasetInfo(meta.CodebaseV30, base.FPS, stripDefaults(base.Features), useVideos, robot)
	outInfo.DataFilesSizeInMB = cfg.DataFileSizeMB
	outInfo.VideoFilesSizeInMB = cfg.VideoFileSizeMB
	outInfo.ChunksSize = cfg.ChunksSize

	taskMap := map[string]int{}
	var allEpMeta []parquetx.EpisodeMetaInput
	var allStats []stats.EpisodeStats
	dataBatch := &dataBatch{chunk: 0, file: 0}
	globalFrame := int64(0)
	globalEp := 0
	tableCache := map[string]arrow.Table{}
	videoStates := map[string]*videoChunkState{}
	tempVideoDir, err := os.MkdirTemp("", "lerobot-go-aggregate-video-*")
	if err != nil {
		return err
	}
	defer func() {
		for _, tbl := range tableCache {
			tbl.Release()
		}
		_ = os.RemoveAll(tempVideoDir)
	}()

	for si, root := range cfg.Inputs {
		srcInfo := infos[si]
		srcTasks, err := loadTasks(ctx, root, srcInfo.CodebaseVersion)
		if err != nil {
			return fmt.Errorf("dataset %q tasks: %w", root, err)
		}
		for task := range srcTasks {
			if _, ok := taskMap[task]; !ok {
				taskMap[task] = len(taskMap)
			}
		}
		srcTaskNames := invertTasks(srcTasks)

		episodes, err := loadEpisodeMetas(root, srcInfo)
		if err != nil {
			return err
		}
		for _, ep := range episodes {
			srcPath, err := episodeDataPath(root, srcInfo, ep)
			if err != nil {
				return err
			}
			srcTable, err := cachedTable(ctx, tableCache, srcPath)
			if err != nil {
				return err
			}
			slice, err := parquetx.SliceTableByIndexRange(srcTable, ep.DatasetFromIndex, ep.DatasetToIndex)
			if err != nil {
				return err
			}
			srcRows := max(1, int(srcTable.NumRows()))
			srcSizeMB, err := meta.ParquetUncompressedSizeMB(srcPath)
			if err != nil {
				slice.Release()
				return err
			}
			estEpisodeMB := srcSizeMB / float64(srcRows) * float64(ep.Length)
			if len(dataBatch.tables) > 0 && dataBatch.sizeMB+estEpisodeMB >= float64(cfg.DataFileSizeMB) {
				if err := flushDataBatch(ctx, cfg.Output, dataBatch); err != nil {
					slice.Release()
					return err
				}
				advanceDataBatch(dataBatch, cfg.ChunksSize)
			}
			n := slice.NumRows()
			indices := make([]int64, n)
			epIdxCol := make([]int64, n)
			taskIdxCol, err := parquetx.ExtractInt64Column(slice, "task_index")
			if err != nil {
				slice.Release()
				return err
			}
			for i := range indices {
				indices[i] = globalFrame + int64(i)
				epIdxCol[i] = int64(globalEp)
			}
			for i, local := range taskIdxCol {
				taskIdxCol[i] = int64(globalTaskIndex(srcTaskNames, local, taskMap))
			}
			step1, err := parquetx.ReplaceInt64Column(slice, "index", indices, nil)
			slice.Release()
			if err != nil {
				return err
			}
			step2, err := parquetx.ReplaceInt64Column(step1, "episode_index", epIdxCol, nil)
			step1.Release()
			if err != nil {
				return err
			}
			step3, err := parquetx.ReplaceInt64Column(step2, "task_index", taskIdxCol, nil)
			step2.Release()
			if err != nil {
				return err
			}

			dataBatch.tables = append(dataBatch.tables, step3)
			dataBatch.rowNums = append(dataBatch.rowNums, n)
			dataBatch.sizeMB += estEpisodeMB

			fromIndex := globalFrame
			globalFrame += n
			fields := map[string]any{
				"data/chunk_index":          dataBatch.chunk,
				"data/file_index":           dataBatch.file,
				"dataset_from_index":        fromIndex,
				"dataset_to_index":          globalFrame,
				"meta/episodes/chunk_index": 0,
				"meta/episodes/file_index":  0,
			}
			for _, videoKey := range videoFeatureKeys(srcInfo.Features) {
				segmentPath, segmentDuration, segmentSizeMB, err := episodeVideoSegment(ctx, cfg, root, srcInfo, ep, videoKey, tempVideoDir)
				if err != nil {
					return err
				}
				if segmentPath == "" {
					continue
				}
				vs := ensureVideoState(videoStates, videoKey)
				if len(vs.segmentPaths) > 0 && vs.sizeMB+segmentSizeMB >= float64(cfg.VideoFileSizeMB) {
					flushVideoState(videoStates, videoKey)
					advanceVideoState(videoStates, videoKey, cfg.ChunksSize)
				}
				fromTS := vs.duration
				vs.segmentPaths = append(vs.segmentPaths, segmentPath)
				vs.sizeMB += segmentSizeMB
				vs.duration += segmentDuration
				fields["videos/"+videoKey+"/chunk_index"] = vs.chunk
				fields["videos/"+videoKey+"/file_index"] = vs.file
				fields["videos/"+videoKey+"/from_timestamp"] = fromTS
				fields["videos/"+videoKey+"/to_timestamp"] = vs.duration
				if vs.sizeMB >= float64(cfg.VideoFileSizeMB) {
					flushVideoState(videoStates, videoKey)
					advanceVideoState(videoStates, videoKey, cfg.ChunksSize)
				}
			}
			allEpMeta = append(allEpMeta, parquetx.EpisodeMetaInput{
				EpisodeIndex: globalEp,
				Tasks:        ep.Tasks,
				Length:       int(ep.Length),
				Fields:       fields,
				Stats:        ep.Stats,
			})
			if ep.Stats != nil {
				allStats = append(allStats, ep.Stats)
			}
			globalEp++
		}
	}

	if err := flushDataBatch(ctx, cfg.Output, dataBatch); err != nil {
		return err
	}
	flushAllVideoStates(videoStates)
	if err := writeVideoBatches(ctx, cfg, videoStates); err != nil {
		return err
	}

	if err := parquetx.WriteTasksParquet(cfg.Output, taskMap); err != nil {
		return err
	}
	if err := parquetx.WriteEpisodesParquet(cfg.Output, allEpMeta, cfg.DataFileSizeMB); err != nil {
		return err
	}
	agg := stats.AggregateStats(allStats)
	if agg == nil {
		agg = map[string]map[string]any{}
	}
	if err := meta.WriteStats(cfg.Output, stats.ToJSONSerializable(agg)); err != nil {
		return err
	}
	outInfo.TotalEpisodes = globalEp
	outInfo.TotalFrames = int(globalFrame)
	outInfo.TotalTasks = len(taskMap)
	return meta.WriteInfo(cfg.Output, outInfo)
}

func flushDataBatch(ctx context.Context, output string, batch *dataBatch) error {
	if len(batch.tables) == 0 {
		return nil
	}
	dst := filepath.Join(output, meta.DataPath(batch.chunk, batch.file))
	merged, err := parquetx.ConcatTables(nil, batch.tables)
	if err != nil {
		return err
	}
	defer merged.Release()
	if err := parquetx.WriteTable(dst, merged, nil); err != nil {
		return err
	}
	for _, tbl := range batch.tables {
		tbl.Release()
	}
	batch.tables = nil
	batch.rowNums = nil
	batch.sizeMB = 0
	return nil
}

func advanceDataBatch(batch *dataBatch, chunkSize int) {
	batch.chunk, batch.file = meta.UpdateChunkFileIndices(batch.chunk, batch.file, chunkSize)
}

type episodeData struct {
	EpisodeIndex     int
	Length           int64
	Tasks            []string
	DatasetFromIndex int64
	DatasetToIndex   int64
	DataChunkIndex   int64
	DataFileIndex    int64
	Stats            stats.EpisodeStats
	VideoFields      map[string]any
}

func loadEpisodeMetas(root string, info meta.DatasetInfo) ([]episodeData, error) {
	switch info.CodebaseVersion {
	case meta.CodebaseV30:
		rows, err := parquetx.ReadEpisodesMeta(root)
		if err != nil {
			return nil, err
		}
		out := make([]episodeData, len(rows))
		for i, r := range rows {
			out[i] = episodeData{
				EpisodeIndex:     int(r.EpisodeIndex),
				Length:           r.Length,
				Tasks:            r.Tasks,
				DatasetFromIndex: r.DatasetFromIndex,
				DatasetToIndex:   r.DatasetToIndex,
				DataChunkIndex:   r.DataChunkIndex,
				DataFileIndex:    r.DataFileIndex,
				VideoFields:      copyVideoFields(r.VideoFields),
			}
		}
		return out, nil
	case meta.CodebaseV21:
		return loadV21EpisodeMetas(root)
	default:
		return nil, fmt.Errorf("unsupported version %s", info.CodebaseVersion)
	}
}

func copyVideoFields(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func videoFeatureKeys(features map[string]meta.FeatureSpec) []string {
	keys := make([]string, 0)
	for key, spec := range features {
		if spec.DType == "video" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func loadV21EpisodeMetas(root string) ([]episodeData, error) {
	rows, err := jsonutil.ReadJSONL(filepath.Join(root, meta.LegacyEpisodesPath))
	if err != nil {
		return nil, err
	}
	var global int64
	out := make([]episodeData, 0, len(rows))
	for _, row := range rows {
		length := int64(row["length"].(float64))
		out = append(out, episodeData{
			EpisodeIndex:     int(row["episode_index"].(float64)),
			Length:           length,
			Tasks:            toStringList(row["tasks"]),
			DatasetFromIndex: global,
			DatasetToIndex:   global + length,
		})
		global += length
	}
	return out, nil
}

func episodeVideoSegment(ctx context.Context, cfg Config, root string, info meta.DatasetInfo, ep episodeData, videoKey, tempDir string) (string, float64, float64, error) {
	switch info.CodebaseVersion {
	case meta.CodebaseV21:
		src := filepath.Join(root, meta.V21VideoPathFromInfo(info, videoKey, ep.EpisodeIndex))
		if _, err := os.Stat(src); err != nil {
			src = filepath.Join(root, meta.V21VideoPathFromInfo(info, filepath.Base(videoKey), ep.EpisodeIndex))
			if _, err2 := os.Stat(src); err2 != nil {
				return "", 0, 0, nil
			}
		}
		dur, err := video.DurationSeconds(ctx, cfg.Locator, src)
		if err != nil {
			return "", 0, 0, err
		}
		sizeMB, err := video.FileSizeMB(src)
		if err != nil {
			return "", 0, 0, err
		}
		return src, dur, sizeMB, nil
	case meta.CodebaseV30:
		chunkKey := "videos/" + videoKey + "/chunk_index"
		fileKey := "videos/" + videoKey + "/file_index"
		fromKey := "videos/" + videoKey + "/from_timestamp"
		toKey := "videos/" + videoKey + "/to_timestamp"
		chunk := firstFloat(ep.VideoFields[chunkKey])
		file := firstFloat(ep.VideoFields[fileKey])
		fromTS := firstFloat(ep.VideoFields[fromKey])
		toTS := firstFloat(ep.VideoFields[toKey])
		if chunk < 0 || file < 0 || toTS <= fromTS {
			return "", 0, 0, nil
		}
		src := filepath.Join(root, meta.VideoPath(videoKey, int(chunk), int(file)))
		out := filepath.Join(tempDir, fmt.Sprintf("%s-ep%06d.mp4", sanitizeVideoKey(videoKey), ep.EpisodeIndex))
		if err := video.ExtractSegment(ctx, cfg.Locator, src, out, fromTS, toTS, true); err != nil {
			return "", 0, 0, err
		}
		sizeMB, err := video.FileSizeMB(out)
		if err != nil {
			return "", 0, 0, err
		}
		return out, toTS - fromTS, sizeMB, nil
	default:
		return "", 0, 0, fmt.Errorf("unsupported version %s", info.CodebaseVersion)
	}
}

func loadTasks(ctx context.Context, root, version string) (map[string]int, error) {
	switch version {
	case meta.CodebaseV30:
		return parquetx.ReadTasksParquet(ctx, filepath.Join(root, meta.DefaultTasksPath))
	case meta.CodebaseV21:
		rows, err := jsonutil.ReadJSONL(filepath.Join(root, meta.LegacyTasksPath))
		if err != nil {
			return nil, err
		}
		out := map[string]int{}
		for _, row := range rows {
			task, _ := row["task"].(string)
			idx, _ := row["task_index"].(float64)
			out[task] = int(idx)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported version %s", version)
	}
}

func invertTasks(m map[string]int) []string {
	if len(m) == 0 {
		return nil
	}
	maxIdx := 0
	for _, idx := range m {
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	inv := make([]string, maxIdx+1)
	for task, idx := range m {
		if idx >= 0 && idx < len(inv) {
			inv[idx] = task
		}
	}
	return inv
}

func globalTaskIndex(taskNames []string, localIdx int64, global map[string]int) int {
	// Staging parquet uses per-episode local indices; v2.1 on-disk uses dataset-global indices.
	if int(localIdx) >= 0 && int(localIdx) < len(taskNames) && taskNames[int(localIdx)] != "" {
		if idx, ok := global[taskNames[int(localIdx)]]; ok {
			return idx
		}
	}
	for _, idx := range global {
		if int64(idx) == localIdx {
			return int(localIdx)
		}
	}
	return int(localIdx)
}

func episodeDataPath(root string, info meta.DatasetInfo, ep episodeData) (string, error) {
	switch info.CodebaseVersion {
	case meta.CodebaseV30:
		return filepath.Join(root, meta.DataPath(int(ep.DataChunkIndex), int(ep.DataFileIndex))), nil
	case meta.CodebaseV21:
		return filepath.Join(root, meta.V21DataPathFromInfo(info, ep.EpisodeIndex)), nil
	default:
		return "", fmt.Errorf("unsupported version %s", info.CodebaseVersion)
	}
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

func toStringList(v any) []string {
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

func ensureVideoState(states map[string]*videoChunkState, key string) *videoChunkState {
	state, ok := states[key]
	if ok {
		return state
	}
	state = &videoChunkState{key: key}
	states[key] = state
	return state
}

func flushVideoState(states map[string]*videoChunkState, key string) {
	state := ensureVideoState(states, key)
	if len(state.segmentPaths) == 0 {
		return
	}
	state.batches = append(state.batches, videoBatch{
		key:          key,
		chunk:        state.chunk,
		file:         state.file,
		segmentPaths: append([]string(nil), state.segmentPaths...),
	})
	state.segmentPaths = nil
	state.sizeMB = 0
	state.duration = 0
}

func flushAllVideoStates(states map[string]*videoChunkState) {
	for key := range states {
		flushVideoState(states, key)
	}
}

func advanceVideoState(states map[string]*videoChunkState, key string, chunkSize int) {
	state := ensureVideoState(states, key)
	state.chunk, state.file = meta.UpdateChunkFileIndices(state.chunk, state.file, chunkSize)
}

func writeVideoBatches(ctx context.Context, cfg Config, states map[string]*videoChunkState) error {
	for _, state := range states {
		for _, batch := range state.batches {
			dst := filepath.Join(cfg.Output, meta.VideoPath(batch.key, batch.chunk, batch.file))
			if err := video.SafeConcat(ctx, cfg.Locator, batch.segmentPaths, dst, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func hasVideo(features map[string]meta.FeatureSpec) bool {
	for _, f := range features {
		if f.DType == "video" {
			return true
		}
	}
	return false
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

func sanitizeVideoKey(key string) string {
	out := make([]rune, 0, len(key))
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
			out = append(out, r)
		case r >= 'A' && r <= 'Z':
			out = append(out, r)
		case r >= '0' && r <= '9':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}

func stripDefaults(features map[string]meta.FeatureSpec) map[string]meta.FeatureSpec {
	out := make(map[string]meta.FeatureSpec, len(features))
	for k, v := range features {
		if _, def := meta.DefaultFeatures[k]; def {
			continue
		}
		out[k] = v
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
