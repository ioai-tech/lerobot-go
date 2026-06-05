package meta

import (
	"fmt"
	"strings"
)

// EpisodeChunk returns the chunk index for a v2.1 per-episode layout.
func EpisodeChunk(episodeIndex, chunksSize int) int {
	if chunksSize <= 0 {
		chunksSize = DefaultChunkSize
	}
	return episodeIndex / chunksSize
}

func formatIntPlaceholder(template, name, format string, value int) string {
	placeholder := "{" + name + ":" + format + "}"
	if strings.Contains(template, placeholder) {
		return strings.ReplaceAll(template, placeholder, fmt.Sprintf("%"+format, value))
	}
	plain := "{" + name + "}"
	return strings.ReplaceAll(template, plain, fmt.Sprintf("%d", value))
}

// FormatPathTemplate expands LeRobot path templates from info.json.
func FormatPathTemplate(template string, episodeIndex, chunkIndex, fileIndex int, videoKey string) string {
	out := template
	if videoKey != "" {
		out = strings.ReplaceAll(out, "{video_key}", videoKey)
	}
	out = formatIntPlaceholder(out, "episode_index", "06d", episodeIndex)
	out = formatIntPlaceholder(out, "episode_chunk", "03d", EpisodeChunk(episodeIndex, DefaultChunkSize))
	out = formatIntPlaceholder(out, "chunk_index", "03d", chunkIndex)
	out = formatIntPlaceholder(out, "file_index", "03d", fileIndex)
	return out
}

// V21DataPathFromInfo resolves the data parquet path for an episode using info.DataPath.
func V21DataPathFromInfo(info DatasetInfo, episodeIndex int) string {
	if strings.Contains(info.DataPath, "{") {
		chunk := EpisodeChunk(episodeIndex, info.ChunksSize)
		out := info.DataPath
		out = strings.ReplaceAll(out, "{video_key}", "")
		out = formatIntPlaceholder(out, "episode_index", "06d", episodeIndex)
		out = formatIntPlaceholder(out, "episode_chunk", "03d", chunk)
		return out
	}
	return V21DataPath(episodeIndex)
}

// V21VideoPathFromInfo resolves a video path for an episode and camera key.
func V21VideoPathFromInfo(info DatasetInfo, videoKey string, episodeIndex int) string {
	if info.VideoPath == nil {
		return ""
	}
	chunk := EpisodeChunk(episodeIndex, info.ChunksSize)
	out := *info.VideoPath
	out = strings.ReplaceAll(out, "{video_key}", videoKey)
	out = formatIntPlaceholder(out, "episode_index", "06d", episodeIndex)
	out = formatIntPlaceholder(out, "episode_chunk", "03d", chunk)
	return out
}
