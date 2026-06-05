package tempfs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

var MemoryRoots = []string{"/dev/shm"}

type Store struct {
	dir         string
	cleanupDir  string
	fallbackDir string
}

type Config struct {
	EpisodeDir string
	TempRoot   string
}

func New(cfg Config) (*Store, error) {
	if cfg.EpisodeDir == "" {
		return nil, fmt.Errorf("episode dir required")
	}
	diskDir := filepath.Join(cfg.EpisodeDir, "_tmp_frames")
	if cfg.TempRoot != "" {
		dir, err := os.MkdirTemp(cfg.TempRoot, "lerobot-go-*")
		if err != nil {
			return nil, err
		}
		return &Store{dir: dir, cleanupDir: dir, fallbackDir: diskDir}, nil
	}
	for _, root := range MemoryRoots {
		dir, err := tryMemoryRoot(root)
		if err == nil {
			return &Store{dir: dir, cleanupDir: dir, fallbackDir: diskDir}, nil
		}
	}
	if err := os.MkdirAll(diskDir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: diskDir, cleanupDir: diskDir}, nil
}

func (s *Store) WritePNG(rel string, data []byte) error {
	if err := s.write(rel, data); err == nil {
		return nil
	} else if !s.shouldFallback(err) {
		return err
	}
	if err := s.fallbackToDisk(); err != nil {
		return fmt.Errorf("temp store fallback: %w", err)
	}
	return s.write(rel, data)
}

func (s *Store) FramePath(feature string, frameIndex int) string {
	return filepath.Join(s.dir, "images", feature, fmt.Sprintf("frame-%06d.png", frameIndex))
}

func (s *Store) Pattern(feature string) string {
	return filepath.Join(s.dir, "images", feature, "frame-%06d.png")
}

func (s *Store) Cleanup() error {
	if s.cleanupDir == "" {
		return nil
	}
	return os.RemoveAll(s.cleanupDir)
}

func (s *Store) write(rel string, data []byte) error {
	path := filepath.Join(s.dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (s *Store) shouldFallback(err error) bool {
	if s.fallbackDir == "" || s.dir == s.fallbackDir {
		return false
	}
	return errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EDQUOT)
}

func (s *Store) fallbackToDisk() error {
	if s.fallbackDir == "" || s.dir == s.fallbackDir {
		return nil
	}
	if err := os.MkdirAll(s.fallbackDir, 0o755); err != nil {
		return err
	}
	if err := copyTree(s.dir, s.fallbackDir); err != nil {
		return err
	}
	if err := os.RemoveAll(s.dir); err != nil {
		return err
	}
	s.dir = s.fallbackDir
	s.cleanupDir = s.fallbackDir
	s.fallbackDir = ""
	return nil
}

func tryMemoryRoot(root string) (string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", root)
	}
	return os.MkdirTemp(root, "lerobot-go-*")
}

func copyTree(srcRoot, dstRoot string) error {
	return filepath.Walk(srcRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(dstRoot, rel)
		if info.IsDir() {
			return os.MkdirAll(dst, info.Mode())
		}
		return copyFile(path, dst, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
