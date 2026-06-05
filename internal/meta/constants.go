package meta

import "fmt"

const (
	CodebaseV21 = "v2.1"
	CodebaseV30 = "v3.0"

	// Official path templates (lerobot v0.3.3 / v0.5.1 datasets.utils).
	V21DefaultDataPathTemplate  = "data/chunk-{episode_chunk:03d}/episode_{episode_index:06d}.parquet"
	V21DefaultVideoPathTemplate = "videos/chunk-{episode_chunk:03d}/{video_key}/episode_{episode_index:06d}.mp4"
	V30DefaultDataPathTemplate  = "data/chunk-{chunk_index:03d}/file-{file_index:03d}.parquet"
	V30DefaultVideoPathTemplate = "videos/{video_key}/chunk-{chunk_index:03d}/file-{file_index:03d}.mp4"

	DefaultChunkSize         = 1000
	DefaultDataFileSizeInMB  = 100
	DefaultVideoFileSizeInMB = 200
	ChunkFilePattern         = "chunk-%03d/file-%03d"
	InfoPath                 = "meta/info.json"
	StatsPath                = "meta/stats.json"
	DefaultTasksPath         = "meta/tasks.parquet"
	EpisodesDir              = "meta/episodes"
	DataDir                  = "data"
	VideoDir                 = "videos"
	LegacyEpisodesPath       = "meta/episodes.jsonl"
	LegacyEpisodesStatsPath  = "meta/episodes_stats.jsonl"
	LegacyTasksPath          = "meta/tasks.jsonl"
	DefaultImagePath         = "images/%s/episode-%06d/frame-%06d.png"
)

func DataPath(chunk, file int) string {
	return DataDir + "/" + FormatChunkFile(chunk, file) + ".parquet"
}

func VideoPath(videoKey string, chunk, file int) string {
	return VideoDir + "/" + videoKey + "/" + FormatChunkFile(chunk, file) + ".mp4"
}

func EpisodesMetaPath(chunk, file int) string {
	return EpisodesDir + "/" + FormatChunkFile(chunk, file) + ".parquet"
}

func V21DataPath(episode int) string {
	return V21DataPathFromInfo(DatasetInfo{
		DataPath:   V21DefaultDataPathTemplate,
		ChunksSize: DefaultChunkSize,
	}, episode)
}

func V21VideoPath(videoKey string, episode int) string {
	p := V21DefaultVideoPathTemplate
	return V21VideoPathFromInfo(DatasetInfo{VideoPath: &p, ChunksSize: DefaultChunkSize}, videoKey, episode)
}

func FormatChunkFile(chunk, file int) string {
	return fmt.Sprintf("chunk-%03d/file-%03d", chunk, file)
}

// UpdateChunkFileIndices mirrors lerobot.datasets.utils.update_chunk_file_indices.
func UpdateChunkFileIndices(chunkIdx, fileIdx, chunksSize int) (int, int) {
	if fileIdx == chunksSize-1 {
		fileIdx = 0
		chunkIdx++
	} else {
		fileIdx++
	}
	return chunkIdx, fileIdx
}
