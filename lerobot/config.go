package lerobot

import (
	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/stats"
	"github.com/ioai-tech/lerobot-go/internal/video"
)

// Version identifies a LeRobot on-disk layout (v2.1 or v3.0).
type Version int

const (
	VersionUnset Version = 0
	// V21 is LeRobot codebase v2.1 (per-episode parquet, optional mp4 videos).
	V21 Version = 21
	// V30 is LeRobot codebase v3.0 (chunked parquet, tasks.parquet).
	V30 Version = 30
)

func (v Version) String() string {
	switch v {
	case V21:
		return meta.CodebaseV21
	case V30:
		return meta.CodebaseV30
	default:
		return "unknown"
	}
}

// FeatureSpec describes one column in meta/info.json and parquet schema.
type FeatureSpec = meta.FeatureSpec

// StatsMode controls image/video episode statistics during finalize.
type StatsMode int

const (
	// StatsSampled uses the official subsampling heuristic (default).
	StatsSampled StatsMode = iota
	// StatsFull scans every frame when computing image/video stats.
	StatsFull
)

func (m StatsMode) toOptions() stats.Options {
	if m == StatsFull {
		return stats.Options{Mode: stats.ModeFull}
	}
	return stats.Options{Mode: stats.ModeSampled}
}

// FFmpegConfig selects ffmpeg and ffprobe binaries (empty paths use PATH lookup).
type FFmpegConfig struct {
	FFmpegPath  string
	FFprobePath string
}

func (c FFmpegConfig) toVideoConfig() video.Config {
	return video.Config{FFmpegPath: c.FFmpegPath, FFprobePath: c.FFprobePath}
}

// CreateConfig builds a dataset serially: staging under Root/_staging, then merge on Finalize.
type CreateConfig struct {
	Version   Version
	RepoID    string
	Root      string
	TempRoot  string
	FPS       int
	RobotType string
	Features  map[string]FeatureSpec
	UseVideos bool
	VCodec    string
	CRF       int
	FFmpeg    FFmpegConfig
	Streaming bool
	Stats     StatsMode
}

// StagingConfig writes one episode directory (ep_NNNNNN) for parallel ingestion.
type StagingConfig struct {
	Version        Version
	Dir            string
	Episode        int
	TempRoot       string
	FPS            int
	RobotType      string
	Features       map[string]FeatureSpec
	UseVideos      bool
	VCodec         string
	CRF            int
	FFmpeg         FFmpegConfig
	Streaming      bool
	Stats          StatsMode
	H264Remux      bool
	ExternalVideos bool
}

// MergeConfig finalizes completed staging episodes into the on-disk dataset layout.
type MergeConfig struct {
	Version     Version
	StagingRoot string
	OutputRoot  string
	RepoID      string
	RobotType   string
	FPS         int
	Features    map[string]FeatureSpec
	FFmpeg      FFmpegConfig
	MaxWorkers  int
	Stats       StatsMode
}

// Frame is one timestep. Task maps to the task_index column; Values hold feature payloads.
type Frame struct {
	Task   string
	Values map[string]any
}

func (f Frame) toMap() map[string]any {
	m := make(map[string]any, len(f.Values)+1)
	for k, v := range f.Values {
		m[k] = v
	}
	if f.Task != "" {
		m["task"] = f.Task
	}
	return m
}
