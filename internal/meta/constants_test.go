package meta

import "testing"

func TestUpdateChunkFileIndices(t *testing.T) {
	c, f := UpdateChunkFileIndices(0, 999, 1000)
	if c != 1 || f != 0 {
		t.Fatalf("got chunk=%d file=%d", c, f)
	}
	c, f = UpdateChunkFileIndices(0, 0, 1000)
	if c != 0 || f != 1 {
		t.Fatalf("got chunk=%d file=%d", c, f)
	}
}

func TestFormatChunkFile(t *testing.T) {
	if got := FormatChunkFile(1, 2); got != "chunk-001/file-002" {
		t.Fatalf("unexpected %q", got)
	}
}

func TestNewDatasetInfoV21SeparatesChunkAndVideoCounters(t *testing.T) {
	info := NewDatasetInfo(CodebaseV21, 30, nil, false, "")
	if info.TotalChunks == nil || info.TotalVideos == nil {
		t.Fatal("v2.1 should initialize total_chunks and total_videos")
	}
	if info.TotalChunks == info.TotalVideos {
		t.Fatal("total_chunks and total_videos must not share the same pointer")
	}
	*info.TotalChunks = 3
	if *info.TotalVideos != 0 {
		t.Fatalf("total_videos changed with total_chunks: got %d", *info.TotalVideos)
	}
}
