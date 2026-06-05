package lerobot_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCLIValidateConvertMerge(t *testing.T) {
	bin := buildCLIBinary(t)
	v21 := filepath.Join(e2eOutputRoot(t), "v21")
	v30 := filepath.Join(e2eOutputRoot(t), "v30")
	if _, err := os.Stat(v21); err != nil {
		t.Skip("run TestWriteDatasetFormats first")
	}

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(bin, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}

	run("validate", v21)
	run("validate", v30)

	tmpV30 := t.TempDir()
	run("convert", "-i", v21, "-o", tmpV30, "--to", "v3.0")
	run("validate", tmpV30)

	tmpV21 := t.TempDir()
	run("convert", "-i", v30, "-o", tmpV21, "--to", "v2.1")
	run("validate", tmpV21)

	merged := t.TempDir()
	run("merge", "-o", merged, "--to", "v3.0", "-i", v21, "-i", v30)
	run("validate", merged)
}

func buildCLIBinary(t *testing.T) string {
	t.Helper()
	root := moduleRoot(t)
	bin := filepath.Join(t.TempDir(), "lerobot-go")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/lerobot-go")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build cli: %s err=%v", out, err)
	}
	return bin
}

func TestCLIRootHelp(t *testing.T) {
	bin := buildCLIBinary(t)
	out, err := exec.Command(bin, "--help").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(out), "validate") || !contains(string(out), "convert") || !contains(string(out), "create") || !contains(string(out), "merge") {
		t.Fatalf("help missing commands: %s", out)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
