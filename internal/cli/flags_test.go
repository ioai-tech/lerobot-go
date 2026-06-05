package cli_test

import (
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/cli"
	"github.com/ioai-tech/lerobot-go/internal/stats"
	"github.com/ioai-tech/lerobot-go/lerobot"
)

func TestParseVersion(t *testing.T) {
	cases := map[string]lerobot.Version{
		"v2.1": lerobot.V21,
		"v2":   lerobot.V21,
		"2.1":  lerobot.V21,
		"v3.0": lerobot.V30,
		"v3":   lerobot.V30,
		"3.0":  lerobot.V30,
	}
	for in, want := range cases {
		got, err := cli.ParseVersion(in)
		if err != nil || got != want {
			t.Fatalf("%q -> %v err=%v want %v", in, got, err, want)
		}
	}
}

func TestParseStatsMode(t *testing.T) {
	for _, tc := range []struct {
		in   string
		full bool
	}{
		{"", false},
		{"sampled", false},
		{"sample", false},
		{"full", true},
	} {
		opts, err := cli.ParseStatsMode(tc.in)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		gotFull := opts.Mode == stats.ModeFull
		if gotFull != tc.full {
			t.Fatalf("%q full=%v want %v", tc.in, gotFull, tc.full)
		}
	}
}

func TestDedupePaths(t *testing.T) {
	paths, err := cli.DedupePaths([]string{"/a", "/a", "/b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("got %v", paths)
	}
}
