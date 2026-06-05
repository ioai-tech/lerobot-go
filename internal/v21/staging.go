package v21

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ioai-tech/lerobot-go/internal/buffer"
	"github.com/ioai-tech/lerobot-go/internal/features"
	"github.com/ioai-tech/lerobot-go/internal/manifest"
	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/parquetx"
	"github.com/ioai-tech/lerobot-go/internal/stats"
	"github.com/ioai-tech/lerobot-go/internal/tempfs"
	"github.com/ioai-tech/lerobot-go/internal/video"
)

type StagingConfig struct {
	Dir       string
	Episode   int
	FPS       int
	Features  map[string]meta.FeatureSpec
	Locator   video.Locator
	VCodec    string
	CRF       int
	UseVideos bool
	Stats     stats.Options
	TempRoot  string
}

type StagingWriter struct {
	cfg              StagingConfig
	buf              *buffer.EpisodeBuffer
	frameStore       *tempfs.Store
	imageBytes       map[string][][]byte
	videoFrameCounts map[string]int
}

func NewStagingWriter(cfg StagingConfig) (*StagingWriter, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("staging dir required")
	}
	if err := features.ValidateMediaConfig(cfg.Features, cfg.UseVideos); err != nil {
		return nil, err
	}
	if cfg.Locator == nil {
		cfg.Locator = video.NewLocator(video.Config{})
	}
	if cfg.CRF <= 0 {
		cfg.CRF = video.DefaultCRF
	}
	feats := features.MergeWithDefaults(cfg.Features)
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, err
	}
	var frameStore *tempfs.Store
	if hasVideoFeatures(cfg.Features) {
		store, err := tempfs.New(tempfs.Config{EpisodeDir: cfg.Dir, TempRoot: cfg.TempRoot})
		if err != nil {
			return nil, err
		}
		frameStore = store
	}
	return &StagingWriter{
		cfg:              cfg,
		buf:              buffer.New(cfg.Episode, cfg.FPS, feats),
		frameStore:       frameStore,
		imageBytes:       make(map[string][][]byte),
		videoFrameCounts: make(map[string]int),
	}, nil
}

func (w *StagingWriter) AddFrame(ctx context.Context, frame map[string]any) error {
	_ = ctx
	for key, spec := range w.buf.Features {
		if spec.DType != "video" && spec.DType != "image" {
			continue
		}
		if raw, ok := frame[key]; ok {
			if png, ok := raw.([]byte); ok {
				switch spec.DType {
				case "image":
					w.imageBytes[key] = append(w.imageBytes[key], append([]byte(nil), png...))
				case "video":
					if w.frameStore == nil {
						return fmt.Errorf("frame store not initialized for video feature %q", key)
					}
					frameIndex := w.videoFrameCounts[key]
					rel := filepath.Join("images", key, fmt.Sprintf("frame-%06d.png", frameIndex))
					if err := w.frameStore.WritePNG(rel, png); err != nil {
						return err
					}
					w.videoFrameCounts[key] = frameIndex + 1
				}
			}
		}
	}
	return w.buf.AddFrame(frame)
}

func (w *StagingWriter) SaveEpisode(ctx context.Context) (manifest.Episode, error) {
	if w.buf.Size() == 0 {
		return manifest.Episode{}, fmt.Errorf("empty episode")
	}
	schema, err := parquetx.BuildArrowSchema(w.buf.Features)
	if err != nil {
		return manifest.Episode{}, err
	}
	taskSet := uniqueTasks(w.buf.Tasks())
	taskIndices := make([]int64, len(w.buf.Tasks()))
	for i, t := range w.buf.Tasks() {
		taskIndices[i] = int64(indexOf(taskSet, t))
	}
	cols := w.buf.Columns(0, taskIndices)
	for key, spec := range w.buf.Features {
		if spec.DType != "image" {
			continue
		}
		cells := buildImageCells(w.imageBytes[key])
		cols[key] = cells
	}
	pqPath := filepath.Join(w.cfg.Dir, "frames.parquet")
	writer, err := parquetx.NewAppendWriterWithFeatures(pqPath, schema, w.buf.Features)
	if err != nil {
		return manifest.Episode{}, err
	}
	if err := writer.WriteEpisodeColumns(cols, w.buf.Size(), w.buf.Features); err != nil {
		_ = writer.Close()
		return manifest.Episode{}, err
	}
	if err := writer.Close(); err != nil {
		return manifest.Episode{}, err
	}

	featureStats := statsFeatureMap(w.buf.Features)
	epStats := stats.ComputeEpisodeStats(stats.EpisodeInput{
		Columns:    cols,
		FramePaths: w.videoFramePaths(),
		FrameBytes: w.imageBytes,
	}, featureStats, w.cfg.Stats)

	videos := map[string]string{}
	durations := map[string]float64{}
	if w.cfg.UseVideos {
		for key, spec := range w.buf.Features {
			if spec.DType != "video" {
				continue
			}
			if w.videoFrameCounts[key] == 0 {
				continue
			}
			out := filepath.Join(w.cfg.Dir, "videos", sanitizeKey(key)+".mp4")
			pattern := w.frameStore.Pattern(key)
			if err := video.EncodeFromPNGDir(ctx, video.EncodeConfig{
				Locator:    w.cfg.Locator,
				VCodec:     w.cfg.VCodec,
				CRF:        w.cfg.CRF,
				FPS:        w.cfg.FPS,
				PNGPattern: pattern,
				OutputPath: out,
			}); err != nil {
				return manifest.Episode{}, err
			}
			videos[key] = out
			d, _ := video.DurationSeconds(ctx, w.cfg.Locator, out)
			durations[key] = d
		}
	}

	ep := manifest.Episode{
		EpisodeIndex:   w.cfg.Episode,
		Length:         w.buf.Size(),
		Tasks:          taskSet,
		FramesParquet:  "frames.parquet",
		Videos:         videos,
		Stats:          epStats,
		VideoDurations: durations,
	}
	if err := manifest.Write(w.cfg.Dir, ep); err != nil {
		return manifest.Episode{}, err
	}
	w.cleanupFrames()
	return ep, nil
}

func buildImageCells(frames [][]byte) []parquetx.ImageCell {
	cells := make([]parquetx.ImageCell, len(frames))
	for i, b := range frames {
		cells[i] = parquetx.ImageCell{
			Bytes: b,
			Path:  fmt.Sprintf("frame_%06d.jpg", i),
		}
	}
	return cells
}

func (w *StagingWriter) Close() error {
	w.cleanupFrames()
	return nil
}

func (w *StagingWriter) videoFramePaths() map[string][]string {
	out := make(map[string][]string, len(w.videoFrameCounts))
	for key, count := range w.videoFrameCounts {
		paths := make([]string, count)
		for i := 0; i < count; i++ {
			paths[i] = w.frameStore.FramePath(key, i)
		}
		out[key] = paths
	}
	return out
}

func (w *StagingWriter) cleanupFrames() {
	if w.frameStore != nil {
		_ = w.frameStore.Cleanup()
		w.frameStore = nil
	}
}

func uniqueTasks(tasks []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, t := range tasks {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func indexOf(tasks []string, task string) int {
	for i, t := range tasks {
		if t == task {
			return i
		}
	}
	return 0
}

func sanitizeKey(key string) string {
	return filepath.Base(key)
}

func statsFeatureMap(features map[string]meta.FeatureSpec) map[string]stats.FeatureDesc {
	out := make(map[string]stats.FeatureDesc, len(features))
	for k, v := range features {
		out[k] = stats.FeatureDesc{DType: v.DType, Shape: v.Shape}
	}
	return out
}
