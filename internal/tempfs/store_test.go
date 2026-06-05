package tempfs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewUsesConfiguredTempRoot(t *testing.T) {
	episodeDir := t.TempDir()
	tempRoot := t.TempDir()

	store, err := New(Config{EpisodeDir: episodeDir, TempRoot: tempRoot})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Cleanup() }()

	path := store.FramePath("cam", 0)
	if !strings.HasPrefix(path, tempRoot) {
		t.Fatalf("frame path %q not under temp root %q", path, tempRoot)
	}
	if err := store.WritePNG(filepath.Join("images", "cam", "frame-000000.png"), []byte("png")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestNewFallsBackToEpisodeDiskRoot(t *testing.T) {
	episodeDir := t.TempDir()
	prev := MemoryRoots
	MemoryRoots = []string{filepath.Join(t.TempDir(), "missing")}
	defer func() { MemoryRoots = prev }()

	store, err := New(Config{EpisodeDir: episodeDir})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Cleanup() }()

	path := store.FramePath("cam", 0)
	wantPrefix := filepath.Join(episodeDir, "_tmp_frames")
	if !strings.HasPrefix(path, wantPrefix) {
		t.Fatalf("frame path %q not under fallback root %q", path, wantPrefix)
	}
	if err := store.WritePNG(filepath.Join("images", "cam", "frame-000000.png"), []byte("png")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
