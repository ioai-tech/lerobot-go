package formatcheck_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/formatcheck"
)

// realDatasetCase describes a local LeRobot dataset used for integration validation.
type realDatasetCase struct {
	path   string
	expect bool // expected OK under strict validation
}

func TestRealDatasetsStrict(t *testing.T) {
	cases := loadRealDatasetCases(t)
	if len(cases) == 0 {
		t.Skip("no real datasets found; set LEROBOT_REAL_DATASETS or install datasets under ~/Downloads")
	}
	for _, tc := range cases {
		t.Run(filepath.Base(tc.path), func(t *testing.T) {
			rep, err := formatcheck.ValidateWithOptions(tc.path, formatcheck.Options{Strict: true})
			if err != nil {
				t.Fatal(err)
			}
			if rep.OK != tc.expect {
				t.Fatalf("strict validate ok=%v want %v version=%s errors=%v warnings=%v",
					rep.OK, tc.expect, rep.Version, rep.Errors, rep.Warnings)
			}
		})
	}
}

func loadRealDatasetCases(t *testing.T) []realDatasetCase {
	t.Helper()
	if raw := os.Getenv("LEROBOT_REAL_DATASETS"); raw != "" {
		var cases []realDatasetCase
		for _, p := range strings.Split(raw, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			cases = append(cases, realDatasetCase{path: p, expect: true})
		}
		return cases
	}

	downloads := filepath.Join(os.Getenv("HOME"), "Downloads")
	entries, err := os.ReadDir(downloads)
	if err != nil {
		return nil
	}

	// Expected outcomes from manual triage against known-good / known-bad exports.
	expected := map[string]bool{
		"lerobot_dataset":         true, // v2.1, missing stats.json is warning only
		"lerobot_dataset 2":       true,
		"lerobot_dataset 3":       true,
		"lerobot_dataset 4":       true,
		"lerobot_dataset 5":       true,
		"lerobot_dataset 6":       true,
		"lerobot_dataset 7":       true,
		"lerobot_dataset 8":       false, // corrupt frame count
		"lerobot_dataset 9":       true,
		"lerobot_dataset 10":      true,
		"lerobot_dataset 22":      false, // v3.0 stats.json omits video keys (issue #4)
		"lerobot_datasetv2-image": true,
	}

	var cases []realDatasetCase
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "lerobot_dataset") {
			continue
		}
		root := filepath.Join(downloads, e.Name())
		if _, err := os.Stat(filepath.Join(root, "meta", "info.json")); err != nil {
			continue
		}
		exp, ok := expected[e.Name()]
		if !ok {
			exp = true
		}
		cases = append(cases, realDatasetCase{path: root, expect: exp})
	}
	return cases
}
