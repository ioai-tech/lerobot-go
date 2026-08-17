package pool

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
)

func TestDefaultMaxWorkersUsesGOMAXPROCS(t *testing.T) {
	want := runtime.GOMAXPROCS(0)
	if want < 1 {
		want = 1
	}
	if got := DefaultMaxWorkers(); got != want {
		t.Fatalf("DefaultMaxWorkers()=%d want %d (GOMAXPROCS)", got, want)
	}
}

func TestRunBounded(t *testing.T) {
	var n atomic.Int32
	jobs := make([]Job, 8)
	for i := range jobs {
		jobs[i] = func(ctx context.Context) error {
			n.Add(1)
			return nil
		}
	}
	if err := Run(context.Background(), Config{MaxWorkers: 2}, jobs); err != nil {
		t.Fatal(err)
	}
	if n.Load() != 8 {
		t.Fatalf("ran %d jobs", n.Load())
	}
}
