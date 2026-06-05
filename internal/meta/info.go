package meta

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultFeatures are auto-injected columns (lerobot.utils.constants.DEFAULT_FEATURES).
var DefaultFeatures = map[string]FeatureSpec{
	"timestamp":     {DType: "float32", Shape: []int{1}},
	"frame_index":   {DType: "int64", Shape: []int{1}},
	"episode_index": {DType: "int64", Shape: []int{1}},
	"index":         {DType: "int64", Shape: []int{1}},
	"task_index":    {DType: "int64", Shape: []int{1}},
}

type FeatureSpec struct {
	DType string         `json:"dtype"`
	Shape []int          `json:"shape"`
	Names interface{}    `json:"names,omitempty"`
	Info  map[string]any `json:"info,omitempty"`
	Extra map[string]any `json:"-"`
}

func (f FeatureSpec) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	for k, v := range f.Extra {
		out[k] = v
	}
	out["dtype"] = f.DType
	out["shape"] = f.Shape
	if f.Names != nil {
		out["names"] = f.Names
	}
	if len(f.Info) > 0 {
		out["info"] = f.Info
	}
	return json.Marshal(out)
}

func (f *FeatureSpec) UnmarshalJSON(data []byte) error {
	type alias struct {
		DType string         `json:"dtype"`
		Shape []int          `json:"shape"`
		Names interface{}    `json:"names,omitempty"`
		Info  map[string]any `json:"info,omitempty"`
	}
	var base alias
	if err := json.Unmarshal(data, &base); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	extra := make(map[string]any)
	for key, val := range raw {
		switch key {
		case "dtype", "shape", "names", "info":
			continue
		default:
			var decoded any
			if err := json.Unmarshal(val, &decoded); err != nil {
				return err
			}
			extra[key] = decoded
		}
	}
	f.DType = base.DType
	f.Shape = base.Shape
	f.Names = base.Names
	f.Info = base.Info
	if len(extra) > 0 {
		f.Extra = extra
	} else {
		f.Extra = nil
	}
	return nil
}

func (f FeatureSpec) Equal(other FeatureSpec) bool {
	af, err := json.Marshal(f)
	if err != nil {
		return false
	}
	bf, err := json.Marshal(other)
	if err != nil {
		return false
	}
	return string(af) == string(bf)
}

type DatasetInfo struct {
	CodebaseVersion    string                 `json:"codebase_version"`
	FPS                int                    `json:"fps"`
	Features           map[string]FeatureSpec `json:"features"`
	TotalEpisodes      int                    `json:"total_episodes"`
	TotalFrames        int                    `json:"total_frames"`
	TotalTasks         int                    `json:"total_tasks"`
	ChunksSize         int                    `json:"chunks_size"`
	DataFilesSizeInMB  int                    `json:"data_files_size_in_mb"`
	VideoFilesSizeInMB int                    `json:"video_files_size_in_mb"`
	DataPath           string                 `json:"data_path"`
	VideoPath          *string                `json:"video_path,omitempty"`
	RobotType          *string                `json:"robot_type,omitempty"`
	Splits             map[string]string      `json:"splits,omitempty"`
	TotalChunks        *int                   `json:"total_chunks,omitempty"`
	TotalVideos        *int                   `json:"total_videos,omitempty"`
}

func NewDatasetInfo(codebase string, fps int, features map[string]FeatureSpec, useVideos bool, robotType string) DatasetInfo {
	merged := make(map[string]FeatureSpec, len(features)+len(DefaultFeatures))
	for k, v := range features {
		merged[k] = v
	}
	for k, v := range DefaultFeatures {
		merged[k] = v
	}

	var videoPath *string
	dataPath := V30DefaultDataPathTemplate
	if useVideos {
		p := V30DefaultVideoPathTemplate
		videoPath = &p
	}
	if codebase == CodebaseV21 {
		dataPath = V21DefaultDataPathTemplate
		if useVideos {
			p := V21DefaultVideoPathTemplate
			videoPath = &p
		}
	}

	rt := robotType
	info := DatasetInfo{
		CodebaseVersion:    codebase,
		FPS:                fps,
		Features:           merged,
		ChunksSize:         DefaultChunkSize,
		DataFilesSizeInMB:  DefaultDataFileSizeInMB,
		VideoFilesSizeInMB: DefaultVideoFileSizeInMB,
		DataPath:           dataPath,
		VideoPath:          videoPath,
		Splits:             map[string]string{},
	}
	if robotType != "" {
		info.RobotType = &rt
	}
	if codebase == CodebaseV21 {
		chunks := 0
		videos := 0
		info.TotalChunks = &chunks
		info.TotalVideos = &videos
	}
	return info
}

func LoadInfo(root string) (DatasetInfo, error) {
	data, err := os.ReadFile(filepath.Join(root, InfoPath))
	if err != nil {
		return DatasetInfo{}, err
	}
	var info DatasetInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return DatasetInfo{}, err
	}
	return info, nil
}

func WriteInfo(root string, info DatasetInfo) error {
	info.Splits = map[string]string{"train": fmt.Sprintf("0:%d", info.TotalEpisodes)}
	path := filepath.Join(root, InfoPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func WriteStats(root string, stats map[string]map[string]any) error {
	path := filepath.Join(root, StatsPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
