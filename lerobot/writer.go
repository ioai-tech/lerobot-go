package lerobot

import (
	"context"
	"fmt"
	"os"

	"github.com/ioai-tech/lerobot-go/internal/manifest"
	"github.com/ioai-tech/lerobot-go/internal/meta"
	v21 "github.com/ioai-tech/lerobot-go/internal/v21"
	v30 "github.com/ioai-tech/lerobot-go/internal/v30"
	"github.com/ioai-tech/lerobot-go/internal/video"
)

// Dataset is a serial writer: episodes are staged under Root/_staging until Finalize.
type Dataset interface {
	AddFrame(ctx context.Context, frame Frame) error
	SaveEpisode(ctx context.Context) error
	Finalize(ctx context.Context) error
	Root() string
}

// StagingWriter records one episode into a single ep_NNNNNN directory.
type StagingWriter interface {
	AddFrame(ctx context.Context, frame Frame) error
	SetH264Remux(ctx context.Context, tracks map[string][][]byte) error
	AppendRGBVideoFrame(ctx context.Context, key string, frame VideoFrameRGB24) error
	SaveEpisode(ctx context.Context) (*EpisodeManifest, error)
	Close() error
}

// EpisodeManifest summarizes a completed staging episode.
type EpisodeManifest struct {
	EpisodeIndex int
	Length       int
	Tasks        []string
	Dir          string
}

type episodeBackend interface {
	AddFrame(ctx context.Context, frame map[string]any) error
	SetH264Remux(ctx context.Context, tracks map[string][][]byte) error
	AppendRGBVideoFrame(ctx context.Context, key string, frame video.VideoFrameRGB24) error
	SaveEpisode(ctx context.Context) (manifest.Episode, error)
	Close() error
}

type stagingWrapper struct {
	backend episodeBackend
	dir     string
}

func (s *stagingWrapper) AddFrame(ctx context.Context, frame Frame) error {
	return s.backend.AddFrame(ctx, frame.toMap())
}

func (s *stagingWrapper) SetH264Remux(ctx context.Context, tracks map[string][][]byte) error {
	return s.backend.SetH264Remux(ctx, tracks)
}

func (s *stagingWrapper) AppendRGBVideoFrame(ctx context.Context, key string, frame VideoFrameRGB24) error {
	return s.backend.AppendRGBVideoFrame(ctx, key, frame)
}

func (s *stagingWrapper) SaveEpisode(ctx context.Context) (*EpisodeManifest, error) {
	ep, err := s.backend.SaveEpisode(ctx)
	if err != nil {
		return nil, err
	}
	return &EpisodeManifest{
		EpisodeIndex: ep.EpisodeIndex,
		Length:       ep.Length,
		Tasks:        ep.Tasks,
		Dir:          s.dir,
	}, nil
}

func (s *stagingWrapper) Close() error { return s.backend.Close() }

// NewStagingWriter opens a per-episode staging directory for parallel ingestion.
func NewStagingWriter(ctx context.Context, cfg StagingConfig) (StagingWriter, error) {
	_ = ctx
	if cfg.Dir == "" {
		return nil, fmt.Errorf("staging dir required")
	}
	if cfg.Version == VersionUnset {
		cfg.Version = V30
	}
	locator := video.NewLocator(cfg.FFmpeg.toVideoConfig())
	var backend episodeBackend
	switch cfg.Version {
	case V21:
		w, err := v21.NewStagingWriter(v21.StagingConfig{
			Dir: cfg.Dir, TempRoot: cfg.TempRoot, Episode: cfg.Episode, FPS: cfg.FPS, Features: cfg.Features,
			Locator: locator, VCodec: cfg.VCodec, CRF: cfg.CRF, UseVideos: cfg.UseVideos, Stats: cfg.Stats.toOptions(),
			H264Remux: cfg.H264Remux,
		})
		if err != nil {
			return nil, err
		}
		backend = w
	default:
		w, err := v30.NewStagingWriter(v30.StagingConfig{
			Dir: cfg.Dir, TempRoot: cfg.TempRoot, Episode: cfg.Episode, FPS: cfg.FPS, Features: cfg.Features,
			Locator: locator, VCodec: cfg.VCodec, CRF: cfg.CRF, UseVideos: cfg.UseVideos, Streaming: cfg.Streaming, Stats: cfg.Stats.toOptions(),
			H264Remux: cfg.H264Remux,
		})
		if err != nil {
			return nil, err
		}
		backend = w
	}
	return &stagingWrapper{backend: backend, dir: cfg.Dir}, nil
}

type serialDataset struct {
	cfg     CreateConfig
	root    string
	writer  StagingWriter
	episode int
	locator video.Locator
}

// Create starts a serial dataset under cfg.Root (staging in Root/_staging).
func Create(ctx context.Context, cfg CreateConfig) (Dataset, error) {
	if cfg.Root == "" {
		return nil, fmt.Errorf("root required")
	}
	if cfg.Version == VersionUnset {
		cfg.Version = V30
	}
	if err := os.MkdirAll(cfg.Root, 0o755); err != nil {
		return nil, err
	}
	stagingRoot := fmt.Sprintf("%s/_staging", cfg.Root)
	if err := os.MkdirAll(stagingRoot, 0o755); err != nil {
		return nil, err
	}
	stCfg := StagingConfig{
		Version: cfg.Version, Dir: manifest.StagingDir(stagingRoot, 0), TempRoot: cfg.TempRoot, Episode: 0,
		FPS: cfg.FPS, RobotType: cfg.RobotType, Features: cfg.Features,
		UseVideos: cfg.UseVideos, VCodec: cfg.VCodec, CRF: cfg.CRF, FFmpeg: cfg.FFmpeg, Streaming: cfg.Streaming, Stats: cfg.Stats,
	}
	sw, err := NewStagingWriter(ctx, stCfg)
	if err != nil {
		return nil, err
	}
	return &serialDataset{cfg: cfg, root: cfg.Root, writer: sw, episode: 0, locator: video.NewLocator(cfg.FFmpeg.toVideoConfig())}, nil
}

func (d *serialDataset) AddFrame(ctx context.Context, frame Frame) error {
	if err := d.ensureWriter(ctx); err != nil {
		return err
	}
	return d.writer.AddFrame(ctx, frame)
}

func (d *serialDataset) SaveEpisode(ctx context.Context) error {
	if d.writer == nil {
		return fmt.Errorf("no frames in current episode")
	}
	if _, err := d.writer.SaveEpisode(ctx); err != nil {
		return err
	}
	d.episode++
	d.writer = nil
	return nil
}

func (d *serialDataset) ensureWriter(ctx context.Context) error {
	if d.writer != nil {
		return nil
	}
	stagingRoot := fmt.Sprintf("%s/_staging", d.root)
	stCfg := StagingConfig{
		Version: d.cfg.Version, Dir: manifest.StagingDir(stagingRoot, d.episode), TempRoot: d.cfg.TempRoot, Episode: d.episode,
		FPS: d.cfg.FPS, RobotType: d.cfg.RobotType, Features: d.cfg.Features,
		UseVideos: d.cfg.UseVideos, VCodec: d.cfg.VCodec, CRF: d.cfg.CRF, FFmpeg: d.cfg.FFmpeg, Streaming: d.cfg.Streaming, Stats: d.cfg.Stats,
	}
	sw, err := NewStagingWriter(ctx, stCfg)
	if err != nil {
		return err
	}
	d.writer = sw
	return nil
}

func (d *serialDataset) Finalize(ctx context.Context) error {
	if d.writer != nil {
		if err := d.writer.Close(); err != nil {
			return err
		}
	}
	return Merge(ctx, MergeConfig{
		Version: d.cfg.Version, StagingRoot: fmt.Sprintf("%s/_staging", d.root),
		OutputRoot: d.root, RepoID: d.cfg.RepoID, RobotType: d.cfg.RobotType,
		FPS: d.cfg.FPS, Features: d.cfg.Features, FFmpeg: d.cfg.FFmpeg, Stats: d.cfg.Stats,
	})
}

func (d *serialDataset) Root() string { return d.root }

// ValidateOutputIntegrity checks merged dataset has data parquet and video files.
func ValidateOutputIntegrity(root string, features map[string]FeatureSpec) error {
	info, err := meta.LoadInfo(root)
	if err != nil {
		return v30.ValidateOutputIntegrity(root, features)
	}
	switch info.CodebaseVersion {
	case meta.CodebaseV21:
		return v21.ValidateOutputIntegrity(root, info, features)
	default:
		return v30.ValidateOutputIntegrity(root, features)
	}
}

// Merge finalizes completed staging episodes into the official on-disk layout.
func Merge(ctx context.Context, cfg MergeConfig) error {
	if cfg.Version == VersionUnset {
		cfg.Version = V30
	}
	locator := video.NewLocator(cfg.FFmpeg.toVideoConfig())
	switch cfg.Version {
	case V21:
		return v21.Merge(ctx, v21.MergeConfig{
			StagingRoot: cfg.StagingRoot, OutputRoot: cfg.OutputRoot,
			RepoID: cfg.RepoID, RobotType: cfg.RobotType, FPS: cfg.FPS, Features: cfg.Features,
			Locator: locator, Stats: cfg.Stats.toOptions(),
		})
	default:
		return v30.Merge(ctx, v30.MergeConfig{
			StagingRoot: cfg.StagingRoot, OutputRoot: cfg.OutputRoot,
			RobotType: cfg.RobotType, FPS: cfg.FPS, Features: cfg.Features,
			Locator: locator, Stats: cfg.Stats.toOptions(), MaxWorkers: cfg.MaxWorkers,
		})
	}
}
