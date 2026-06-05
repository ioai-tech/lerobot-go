package formatcheck_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/formatcheck"
)

func TestValidateMissingRoot(t *testing.T) {
	_, err := formatcheck.Validate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateOutputDatasets(t *testing.T) {
	root := moduleRoot(t)
	for _, ver := range []string{"v21", "v30"} {
		path := filepath.Join(root, "testdata", "output", ver)
		if _, err := os.Stat(path); err != nil {
			t.Skipf("run TestWriteDatasetFormats first (missing %s)", path)
		}
		rep, err := formatcheck.Validate(path)
		if err != nil {
			t.Fatal(err)
		}
		if !rep.OK {
			t.Fatalf("%s: %v", ver, rep.Errors)
		}
	}
}

func moduleRoot(t *testing.T) string {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("go.mod not found")
		}
		wd = parent
	}
}
