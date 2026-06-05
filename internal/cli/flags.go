package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/stats"
	"github.com/ioai-tech/lerobot-go/lerobot"
)

// GlobalOpts holds persistent CLI flags.
type GlobalOpts struct {
	Verbose    bool
	FFmpegPath string
	JSON       bool
}

func CleanPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[2:])
	}
	return filepath.Clean(path), nil
}

func statsModeFromOpts(opts stats.Options) lerobot.StatsMode {
	if opts.Mode == stats.ModeFull {
		return lerobot.StatsFull
	}
	return lerobot.StatsSampled
}

func ParseStatsMode(s string) (stats.Options, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "sampled", "sample":
		return stats.Options{Mode: stats.ModeSampled}, nil
	case "full":
		return stats.Options{Mode: stats.ModeFull}, nil
	default:
		return stats.Options{}, fmt.Errorf("unsupported stats mode %q (want sampled or full)", s)
	}
}

func ParseVersion(s string) (lerobot.Version, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "v2.1", "v2", "2.1":
		return lerobot.V21, nil
	case "v3.0", "v3", "3.0":
		return lerobot.V30, nil
	default:
		return lerobot.VersionUnset, fmt.Errorf("unsupported version %q (want v2.1 or v3.0)", s)
	}
}

func VersionString(v lerobot.Version) string {
	switch v {
	case lerobot.V21:
		return meta.CodebaseV21
	case lerobot.V30:
		return meta.CodebaseV30
	default:
		return "unknown"
	}
}

func EnsureDatasetRoot(root string) error {
	clean, err := CleanPath(root)
	if err != nil {
		return err
	}
	info, err := os.Stat(filepath.Join(clean, meta.InfoPath))
	if err != nil {
		return fmt.Errorf("%q: missing meta/info.json: %w", clean, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%q: meta/info.json is a directory", clean)
	}
	return nil
}

func EnsureOutputDir(path string, force bool) error {
	clean, err := CleanPath(path)
	if err != nil {
		return err
	}
	fi, err := os.Stat(clean)
	if os.IsNotExist(err) {
		return os.MkdirAll(clean, 0o755)
	}
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("%q: output exists and is not a directory", clean)
	}
	entries, err := os.ReadDir(clean)
	if err != nil {
		return err
	}
	if len(entries) > 0 && !force {
		return fmt.Errorf("%q: output directory not empty (use --force)", clean)
	}
	return nil
}

func DedupePaths(paths []string) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		clean, err := CleanPath(p)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out, nil
}

func Logf(g GlobalOpts, format string, args ...any) {
	if g.Verbose {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

func PrintJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func PrintFileTree(root string) error {
	clean, err := CleanPath(root)
	if err != nil {
		return err
	}
	var files []string
	_ = filepath.Walk(clean, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(clean, path)
		files = append(files, rel)
		return nil
	})
	fmt.Fprintf(os.Stderr, "%s (%d files):\n", clean, len(files))
	for _, f := range files {
		fmt.Fprintf(os.Stderr, "  %s\n", f)
	}
	return nil
}

func LoadVersionFromRoot(root string) (lerobot.Version, error) {
	clean, err := CleanPath(root)
	if err != nil {
		return lerobot.VersionUnset, err
	}
	info, err := meta.LoadInfo(clean)
	if err != nil {
		return lerobot.VersionUnset, err
	}
	return ParseVersion(info.CodebaseVersion)
}
