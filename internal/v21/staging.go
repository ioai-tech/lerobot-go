package v21

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

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
	Dir            string
	Episode        int
	FPS            int
	Features       map[string]meta.FeatureSpec
	Locator        video.Locator
	VCodec         string
	CRF            int
	UseVideos      bool
	Stats          stats.Options
	TempRoot       string
	H264Remux      bool
	ExternalVideos bool
}

type StagingWriter struct {
	cfg              StagingConfig
	buf              *buffer.EpisodeBuffer
	frameStore       *tempfs.Store
	imageBytes       map[string][][]byte
	videoFrameCounts map[string]int
	videoEncoders    map[string]*video.RawRGBEncoder
	pendingH264Remux map[string][][]byte
	externalVideos   map[string]string
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
	if hasVideoFeatures(cfg.Features) && !cfg.UseVideos {
		store, err := tempfs.New(tempfs.Config{EpisodeDir: cfg.Dir, TempRoot: cfg.TempRoot})
		if err != nil {
			return nil, err
		}
		frameStore = store
	}
	videoEncoders := make(map[string]*video.RawRGBEncoder)
	if cfg.UseVideos && !cfg.H264Remux && !cfg.ExternalVideos {
		for key, spec := range feats {
			if spec.DType == "video" && len(spec.Shape) >= 2 {
				h, w := spec.Shape[0], spec.Shape[1]
				rel := manifest.StagingVideoRel(key)
				out := filepath.Join(cfg.Dir, rel)
				if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
					for _, e := range videoEncoders {
						_ = e.Close()
					}
					return nil, err
				}
				enc, err := video.NewRawRGBEncoder(context.Background(), cfg.Locator, cfg.VCodec, cfg.CRF, cfg.FPS, w, h, out)
				if err != nil {
					for _, e := range videoEncoders {
						_ = e.Close()
					}
					return nil, err
				}
				videoEncoders[key] = enc
			}
		}
	}
	return &StagingWriter{
		cfg:              cfg,
		buf:              buffer.New(cfg.Episode, cfg.FPS, feats),
		frameStore:       frameStore,
		imageBytes:       make(map[string][][]byte),
		videoFrameCounts: make(map[string]int),
		videoEncoders:    videoEncoders,
	}, nil
}

func (w *StagingWriter) SetH264Remux(ctx context.Context, tracks map[string][][]byte) error {
	_ = ctx
	if !w.cfg.H264Remux {
		return fmt.Errorf("staging writer not configured for h264 remux")
	}
	w.pendingH264Remux = tracks
	return nil
}

func (w *StagingWriter) SetVideoFiles(ctx context.Context, files map[string]string) error {
	_ = ctx
	if w.externalVideos == nil {
		w.externalVideos = make(map[string]string, len(files))
	}
	for key, path := range files {
		spec, ok := w.buf.Features[key]
		if !ok || spec.DType != "video" {
			return fmt.Errorf("not a video feature: %q", key)
		}
		if path == "" {
			return fmt.Errorf("empty video path for %q", key)
		}
		w.externalVideos[key] = path
	}
	return nil
}

func (w *StagingWriter) AppendRGBVideoFrame(ctx context.Context, key string, frame video.VideoFrameRGB24) error {
	spec, ok := w.buf.Features[key]
	if !ok || spec.DType != "video" {
		return fmt.Errorf("not a video feature: %q", key)
	}
	if err := frame.Validate(); err != nil {
		return err
	}
	enc, err := w.ensureRawEncoder(key, spec)
	if err != nil {
		return err
	}
	if err := enc.WriteFrame(frame); err != nil {
		return err
	}
	w.videoFrameCounts[key]++
	return nil
}

func (w *StagingWriter) AddFrame(ctx context.Context, frame map[string]any) error {
	_ = ctx
	usedDecodeEncoder := false
	for key, spec := range w.buf.Features {
		if spec.DType != "video" && spec.DType != "image" {
			continue
		}
		val, ok := frame[key]
		if !ok {
			continue
		}
		switch spec.DType {
		case "image":
			if png, ok := val.([]byte); ok {
				w.imageBytes[key] = append(w.imageBytes[key], append([]byte(nil), png...))
			}
		case "video":
			if len(spec.Shape) < 2 {
				continue
			}
			vf, ok, err := video.ParseOptionalVideoFrameRGB24(val, spec.Shape[1], spec.Shape[0])
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			enc, err := w.ensureRawEncoder(key, spec)
			if err != nil {
				return err
			}
			if err := enc.WriteFrame(vf); err != nil {
				return err
			}
			w.videoFrameCounts[key]++
			usedDecodeEncoder = true
		}
	}
	if !usedDecodeEncoder && w.cfg.H264Remux && w.cfg.UseVideos {
		for key, spec := range w.buf.Features {
			if spec.DType == "video" {
				w.videoFrameCounts[key]++
			}
		}
	}
	return w.buf.AddFrame(frame)
}

func (w *StagingWriter) ensureRawEncoder(key string, spec meta.FeatureSpec) (*video.RawRGBEncoder, error) {
	if enc := w.videoEncoders[key]; enc != nil {
		return enc, nil
	}
	if len(spec.Shape) < 2 {
		return nil, fmt.Errorf("video feature %q missing shape", key)
	}
	h, width := spec.Shape[0], spec.Shape[1]
	rel := manifest.StagingVideoRel(key)
	out := filepath.Join(w.cfg.Dir, rel)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return nil, err
	}
	enc, err := video.NewRawRGBEncoder(context.Background(), w.cfg.Locator, w.cfg.VCodec, w.cfg.CRF, w.cfg.FPS, width, h, out)
	if err != nil {
		return nil, err
	}
	if w.videoEncoders == nil {
		w.videoEncoders = make(map[string]*video.RawRGBEncoder)
	}
	w.videoEncoders[key] = enc
	return enc, nil
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

	videos, durations, err := w.finalizeVideos(ctx)
	if err != nil {
		return manifest.Episode{}, err
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

func (w *StagingWriter) videoDurationFromFrames(key string) float64 {
	if w.cfg.FPS <= 0 {
		return 0
	}
	return float64(w.videoFrameCounts[key]) / float64(w.cfg.FPS)
}

func (w *StagingWriter) finalizeVideos(ctx context.Context) (map[string]string, map[string]float64, error) {
	videos := map[string]string{}
	durations := map[string]float64{}
	if !w.cfg.UseVideos {
		return videos, durations, nil
	}
	xv, xd, err := w.finalizeExternalVideos(ctx)
	if err != nil {
		return nil, nil, err
	}
	for k, v := range xv {
		videos[k] = v
	}
	for k, v := range xd {
		durations[k] = v
	}
	if w.cfg.H264Remux {
		remuxVideos, remuxDurations, err := w.finalizeH264RemuxVideos(ctx)
		if err != nil {
			return nil, nil, err
		}
		if len(remuxVideos) > 0 {
			return remuxVideos, remuxDurations, nil
		}
		// Decode-fallback path may have streamed RGB into per-episode encoders without SetH264Remux.
	}
	type encClose struct {
		key string
		enc *video.RawRGBEncoder
	}
	var toClose []encClose
	for key, spec := range w.buf.Features {
		if spec.DType != "video" || w.videoFrameCounts[key] == 0 {
			continue
		}
		if _, external := w.externalVideos[key]; external {
			continue
		}
		if enc := w.videoEncoders[key]; enc != nil {
			toClose = append(toClose, encClose{key: key, enc: enc})
		}
	}
	for _, job := range toClose {
		if err := job.enc.Close(); err != nil {
			return nil, nil, err
		}
	}
	for key, spec := range w.buf.Features {
		if spec.DType != "video" || w.videoFrameCounts[key] == 0 {
			continue
		}
		if _, external := w.externalVideos[key]; external {
			continue
		}
		rel := manifest.StagingVideoRel(key)
		out := filepath.Join(w.cfg.Dir, rel)
		if enc := w.videoEncoders[key]; enc != nil {
			videos[key] = rel
			durations[key] = w.videoDurationFromFrames(key)
			continue
		}
		if w.frameStore == nil {
			continue
		}
		if err := video.EncodeFromPNGDir(ctx, video.EncodeConfig{
			Locator:    w.cfg.Locator,
			VCodec:     w.cfg.VCodec,
			CRF:        w.cfg.CRF,
			FPS:        w.cfg.FPS,
			Threads:    video.ResolveEncoderThreads(),
			PNGPattern: w.frameStore.Pattern(key),
			OutputPath: out,
		}); err != nil {
			return nil, nil, err
		}
		videos[key] = rel
		if os.Getenv("LEROBOT_FFPROBE_DURATION") == "1" {
			d, _ := video.DurationSeconds(ctx, w.cfg.Locator, out)
			durations[key] = d
		} else {
			durations[key] = w.videoDurationFromFrames(key)
		}
	}
	return videos, durations, nil
}

func (w *StagingWriter) finalizeExternalVideos(ctx context.Context) (map[string]string, map[string]float64, error) {
	videos := map[string]string{}
	durations := map[string]float64{}
	for key, src := range w.externalVideos {
		spec, ok := w.buf.Features[key]
		if !ok || spec.DType != "video" {
			continue
		}
		rel := manifest.StagingVideoRel(key)
		dst := filepath.Join(w.cfg.Dir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, nil, err
		}
		if err := copyExternalVideo(src, dst); err != nil {
			return nil, nil, err
		}
		w.videoFrameCounts[key] = w.buf.Size()
		if err := video.ValidateMP4(ctx, w.cfg.Locator, dst, w.videoFrameCounts[key], w.cfg.FPS); err != nil {
			return nil, nil, err
		}
		videos[key] = rel
		durations[key] = w.videoDurationFromFrames(key)
	}
	return videos, durations, nil
}

func (w *StagingWriter) finalizeH264RemuxVideos(ctx context.Context) (map[string]string, map[string]float64, error) {
	videos := map[string]string{}
	durations := map[string]float64{}
	if w.pendingH264Remux == nil {
		return videos, durations, nil
	}
	type remuxJob struct {
		key string
		aus [][]byte
		rel string
		out string
	}
	var jobs []remuxJob
	for key, spec := range w.buf.Features {
		if spec.DType != "video" {
			continue
		}
		aus, ok := w.pendingH264Remux[key]
		if !ok || len(aus) == 0 {
			continue
		}
		rel := manifest.StagingVideoRel(key)
		out := filepath.Join(w.cfg.Dir, rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return nil, nil, err
		}
		jobs = append(jobs, remuxJob{key: key, aus: aus, rel: rel, out: out})
	}
	if len(jobs) == 0 {
		return videos, durations, nil
	}
	var countMu sync.Mutex
	runJob := func(job remuxJob) error {
		enc, err := video.NewH264RemuxEncoder(ctx, w.cfg.Locator, w.cfg.FPS, job.out)
		if err != nil {
			return err
		}
		if err := enc.WriteAccessUnits(job.aus); err != nil {
			_ = enc.Close()
			return err
		}
		if err := enc.Close(); err != nil {
			return err
		}
		expected := len(job.aus)
		if nb, err := enc.FrameCount(ctx); err == nil && expected > 0 {
			if nb != expected && nb+1 != expected && nb != expected-1 {
				slog.Warn("h264 remux frame count drift", "feature", job.key, "ffprobe", nb, "expected", expected)
			}
		}
		countMu.Lock()
		w.videoFrameCounts[job.key] = expected
		countMu.Unlock()
		return nil
	}
	if len(jobs) == 1 {
		if err := runJob(jobs[0]); err != nil {
			return nil, nil, err
		}
		videos[jobs[0].key] = jobs[0].rel
		durations[jobs[0].key] = w.videoDurationFromFrames(jobs[0].key)
		return videos, durations, nil
	}
	var wg sync.WaitGroup
	errCh := make(chan error, len(jobs))
	for _, job := range jobs {
		wg.Add(1)
		go func(job remuxJob) {
			defer wg.Done()
			if err := runJob(job); err != nil {
				errCh <- err
			}
		}(job)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return nil, nil, err
		}
	}
	for _, job := range jobs {
		videos[job.key] = job.rel
		durations[job.key] = w.videoDurationFromFrames(job.key)
	}
	return videos, durations, nil
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
	for _, enc := range w.videoEncoders {
		_ = enc.Close()
	}
	w.cleanupFrames()
	return nil
}

func (w *StagingWriter) videoFramePaths() map[string][]string {
	if w.frameStore == nil {
		return nil
	}
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

func copyExternalVideo(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
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

func statsFeatureMap(features map[string]meta.FeatureSpec) map[string]stats.FeatureDesc {
	out := make(map[string]stats.FeatureDesc, len(features))
	for k, v := range features {
		out[k] = stats.FeatureDesc{DType: v.DType, Shape: v.Shape}
	}
	return out
}
