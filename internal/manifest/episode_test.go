package manifest

import (
	"path/filepath"
	"testing"
)

func TestStagingMediaPath(t *testing.T) {
	dir := "/tmp/ep_000000"
	rel := StagingVideoRel("observation.images.cam")
	if rel != filepath.Join("videos", "observation.images.cam.mp4") {
		t.Fatalf("rel=%q", rel)
	}
	got := StagingMediaPath(dir, rel)
	want := filepath.Join(dir, rel)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	abs := "/tmp/ep_000000/videos/foo.mp4"
	if StagingMediaPath(dir, abs) != abs {
		t.Fatalf("absolute path should pass through")
	}
}
