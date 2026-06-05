package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ioai-tech/lerobot-go/internal/stats"
)

type Episode struct {
	EpisodeIndex   int                `json:"episode_index"`
	Length         int                `json:"length"`
	Tasks          []string           `json:"tasks"`
	FramesParquet  string             `json:"frames_parquet"`
	Videos         map[string]string  `json:"videos"`
	Stats          stats.EpisodeStats `json:"stats"`
	VideoDurations map[string]float64 `json:"video_durations"`
}

const FileName = "episode_meta.json"

func Write(dir string, ep Episode) error {
	data, err := json.MarshalIndent(ep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, FileName), data, 0o644)
}

func Read(dir string) (Episode, error) {
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		return Episode{}, err
	}
	var ep Episode
	if err := json.Unmarshal(data, &ep); err != nil {
		return Episode{}, err
	}
	return ep, nil
}

func StagingDir(root string, episodeIndex int) string {
	return filepath.Join(root, stagingName(episodeIndex))
}

func stagingName(episodeIndex int) string {
	return fmt.Sprintf("ep_%06d", episodeIndex)
}

// ListStagingEpisodes returns completed staging episode dirs sorted by episode_index.
func ListStagingEpisodes(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "ep_") {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(dir, FileName)); err != nil {
			continue
		}
		dirs = append(dirs, dir)
	}
	sort.Slice(dirs, func(i, j int) bool {
		ei, _ := Read(dirs[i])
		ej, _ := Read(dirs[j])
		return ei.EpisodeIndex < ej.EpisodeIndex
	})
	return dirs, nil
}
