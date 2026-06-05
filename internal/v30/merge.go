package v30

import (
	"context"
	"fmt"

	"path/filepath"

	"github.com/ioai-tech/lerobot-go/internal/manifest"
	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/parquetx"
	"github.com/ioai-tech/lerobot-go/internal/stagingstats"
	"github.com/ioai-tech/lerobot-go/internal/stats"
	"github.com/ioai-tech/lerobot-go/internal/video"
)

type MergeConfig struct {
	StagingRoot     string
	OutputRoot      string
	RobotType       string
	FPS             int
	Features        map[string]meta.FeatureSpec
	Locator         video.Locator
	Stats           stats.Options
	DataFileSizeMB  int
	VideoFileSizeMB int
}

type mergeState struct {
	info             meta.DatasetInfo
	taskMap          map[string]int
	globalFrameIndex int64
	dataBatch        dataBatch
	dataBatches      []dataBatch
	videoState       map[string]*videoChunkState
	allEpisodeStats  []stats.EpisodeStats
	episodesMeta     []parquetx.EpisodeMetaInput
	episodesChunk    int
	episodesFile     int
}

type dataBatch struct {
	chunk   int
	file    int
	sizeMB  float64
	entries []parquetx.EpisodeBatchEntry
}

type videoBatch struct {
	key         string
	chunk       int
	file        int
	segmentPath []string
}

type videoChunkState struct {
	key          string
	chunk        int
	file         int
	sizeMB       float64
	segmentPaths []string
	duration     float64
	batches      []videoBatch
}

func Merge(ctx context.Context, cfg MergeConfig) error {
	if cfg.Locator == nil {
		cfg.Locator = video.NewLocator(video.Config{})
	}
	dirs, err := manifest.ListStagingEpisodes(cfg.StagingRoot)
	if err != nil {
		return err
	}
	if len(dirs) == 0 {
		return fmt.Errorf("no staging episodes")
	}
	useVideos := hasVideoFeatures(cfg.Features)
	st := &mergeState{
		info:          meta.NewDatasetInfo(meta.CodebaseV30, cfg.FPS, cfg.Features, useVideos, cfg.RobotType),
		dataBatch:     dataBatch{chunk: 0, file: 0},
		taskMap:       map[string]int{},
		videoState:    map[string]*videoChunkState{},
		episodesChunk: 0,
		episodesFile:  0,
	}
	if cfg.DataFileSizeMB > 0 {
		st.info.DataFilesSizeInMB = cfg.DataFileSizeMB
	}
	if cfg.VideoFileSizeMB > 0 {
		st.info.VideoFilesSizeInMB = cfg.VideoFileSizeMB
	}
	for _, dir := range dirs {
		ep, err := manifest.Read(dir)
		if err != nil {
			return err
		}
		for _, t := range ep.Tasks {
			if _, ok := st.taskMap[t]; !ok {
				st.taskMap[t] = len(st.taskMap)
			}
		}
	}
	for _, dir := range dirs {
		ep, err := manifest.Read(dir)
		if err != nil {
			return err
		}
		if err := st.ingestEpisode(ctx, cfg, dir, ep); err != nil {
			return err
		}
	}
	st.flushDataBatch()
	st.flushVideoBatches()
	if err := st.writeDataBatches(ctx, cfg.OutputRoot); err != nil {
		return err
	}
	if err := st.writeVideoBatches(ctx, cfg); err != nil {
		return err
	}
	if err := parquetx.WriteTasksParquet(cfg.OutputRoot, st.taskMap); err != nil {
		return err
	}
	if err := parquetx.WriteEpisodesParquet(cfg.OutputRoot, st.episodesMeta, st.info.DataFilesSizeInMB); err != nil {
		return err
	}
	agg := stats.AggregateStats(st.allEpisodeStats)
	if err := meta.WriteStats(cfg.OutputRoot, stats.ToJSONSerializable(agg)); err != nil {
		return err
	}
	st.info.TotalTasks = len(st.taskMap)
	if err := meta.UpdateVideoFeaturesInfo(ctx, &st.info, cfg.OutputRoot, cfg.Locator); err != nil {
		return err
	}
	return meta.WriteInfo(cfg.OutputRoot, st.info)
}

func (st *mergeState) ingestEpisode(ctx context.Context, cfg MergeConfig, dir string, ep manifest.Episode) error {
	srcPQ := filepath.Join(dir, ep.FramesParquet)
	srcSizeMB, err := meta.ParquetUncompressedSizeMB(srcPQ)
	if err != nil {
		return err
	}
	st.maybeRotateDataBatch(srcSizeMB)
	dataChunk := st.dataBatch.chunk
	dataFile := st.dataBatch.file
	appendOpts := parquetx.AppendEpisodeOptions{
		GlobalFrameIndex: st.globalFrameIndex,
		EpisodeIndex:     int64(ep.EpisodeIndex),
		EpisodeTasks:     ep.Tasks,
		GlobalTaskIndex:  st.taskMap,
	}
	st.dataBatch.entries = append(st.dataBatch.entries, parquetx.EpisodeBatchEntry{SourcePath: srcPQ, Options: appendOpts})
	st.dataBatch.sizeMB += srcSizeMB

	fromIndex := st.globalFrameIndex
	toIndex := st.globalFrameIndex + int64(ep.Length)
	epFields := map[string]any{
		"data/chunk_index":          dataChunk,
		"data/file_index":           dataFile,
		"dataset_from_index":        fromIndex,
		"dataset_to_index":          toIndex,
		"meta/episodes/chunk_index": st.episodesChunk,
		"meta/episodes/file_index":  st.episodesFile,
	}
	for videoKey, rel := range ep.Videos {
		vs := st.ensureVideoState(videoKey)
		src := filepath.Join(dir, rel)
		segSizeMB, err := video.FileSizeMB(src)
		if err != nil {
			return err
		}
		if len(vs.segmentPaths) > 0 && vs.sizeMB+segSizeMB >= float64(st.info.VideoFilesSizeInMB) {
			st.flushVideoState(videoKey)
			st.advanceVideoState(videoKey)
		}
		vs.segmentPaths = append(vs.segmentPaths, src)
		videoChunk := vs.chunk
		videoFile := vs.file
		fromTS := vs.duration
		vs.duration += ep.VideoDurations[videoKey]
		vs.sizeMB += segSizeMB
		epFields["videos/"+videoKey+"/chunk_index"] = videoChunk
		epFields["videos/"+videoKey+"/file_index"] = videoFile
		epFields["videos/"+videoKey+"/from_timestamp"] = fromTS
		epFields["videos/"+videoKey+"/to_timestamp"] = vs.duration
		if vs.sizeMB >= float64(st.info.VideoFilesSizeInMB) {
			st.flushVideoState(videoKey)
			st.advanceVideoState(videoKey)
		}
	}
	epStats := ep.Stats
	if recomputed, err := stagingstats.RecomputeEpisodeStatsWithOptions(ctx, dir, ep, cfg.Features, cfg.Stats, &appendOpts); err == nil && len(recomputed) > 0 {
		epStats = mergeEpisodeStats(epStats, recomputed)
	}
	st.episodesMeta = append(st.episodesMeta, parquetx.EpisodeMetaInput{
		EpisodeIndex: ep.EpisodeIndex,
		Tasks:        ep.Tasks,
		Length:       ep.Length,
		Fields:       epFields,
		Stats:        epStats,
		Features:     st.info.Features,
	})
	st.allEpisodeStats = append(st.allEpisodeStats, epStats)
	st.globalFrameIndex = toIndex
	st.info.TotalEpisodes++
	st.info.TotalFrames += ep.Length
	if st.dataBatch.sizeMB >= float64(st.info.DataFilesSizeInMB) {
		st.flushDataBatch()
		st.advanceDataBatch()
	}
	return nil
}

func (st *mergeState) maybeRotateDataBatch(srcSizeMB float64) {
	if len(st.dataBatch.entries) == 0 {
		return
	}
	if st.dataBatch.sizeMB+srcSizeMB >= float64(st.info.DataFilesSizeInMB) {
		st.flushDataBatch()
		st.advanceDataBatch()
	}
}

func (st *mergeState) flushDataBatch() {
	if len(st.dataBatch.entries) == 0 {
		return
	}
	batch := dataBatch{
		chunk:   st.dataBatch.chunk,
		file:    st.dataBatch.file,
		sizeMB:  st.dataBatch.sizeMB,
		entries: append([]parquetx.EpisodeBatchEntry(nil), st.dataBatch.entries...),
	}
	st.dataBatches = append(st.dataBatches, batch)
	st.dataBatch.entries = nil
	st.dataBatch.sizeMB = 0
}

func (st *mergeState) advanceDataBatch() {
	st.dataBatch.chunk, st.dataBatch.file = meta.UpdateChunkFileIndices(st.dataBatch.chunk, st.dataBatch.file, st.info.ChunksSize)
}

func (st *mergeState) ensureVideoState(videoKey string) *videoChunkState {
	vs, ok := st.videoState[videoKey]
	if ok {
		return vs
	}
	vs = &videoChunkState{key: videoKey}
	st.videoState[videoKey] = vs
	return vs
}

func (st *mergeState) flushVideoState(videoKey string) {
	vs := st.ensureVideoState(videoKey)
	if len(vs.segmentPaths) == 0 {
		return
	}
	batch := videoBatch{
		key:         videoKey,
		chunk:       vs.chunk,
		file:        vs.file,
		segmentPath: append([]string(nil), vs.segmentPaths...),
	}
	vs.batches = append(vs.batches, batch)
	vs.segmentPaths = nil
	vs.sizeMB = 0
	vs.duration = 0
}

func (st *mergeState) advanceVideoState(videoKey string) {
	vs := st.ensureVideoState(videoKey)
	vs.chunk, vs.file = meta.UpdateChunkFileIndices(vs.chunk, vs.file, st.info.ChunksSize)
}

func (st *mergeState) flushVideoBatches() {
	for key := range st.videoState {
		st.flushVideoState(key)
	}
}

func (st *mergeState) writeDataBatches(ctx context.Context, outputRoot string) error {
	for _, batch := range st.dataBatches {
		dst := filepath.Join(outputRoot, meta.DataPath(batch.chunk, batch.file))
		if err := parquetx.WriteEpisodeBatch(ctx, dst, batch.entries); err != nil {
			return err
		}
	}
	return nil
}

func (st *mergeState) writeVideoBatches(ctx context.Context, cfg MergeConfig) error {
	for _, state := range st.videoState {
		for _, batch := range state.batches {
			dst := filepath.Join(cfg.OutputRoot, meta.VideoPath(batch.key, batch.chunk, batch.file))
			if err := video.SafeConcat(ctx, cfg.Locator, batch.segmentPath, dst, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func hasVideoFeatures(features map[string]meta.FeatureSpec) bool {
	for _, f := range features {
		if f.DType == "video" {
			return true
		}
	}
	return false
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
