package pool

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"golang.org/x/sync/errgroup"
)

type Job func(ctx context.Context) error

type Config struct {
	MaxWorkers int
}

func DefaultMaxWorkers() int {
	n := runtime.NumCPU() - 2
	if n < 1 {
		n = 1
	}
	return n
}

func Run(ctx context.Context, cfg Config, jobs []Job) error {
	if len(jobs) == 0 {
		return nil
	}
	maxW := cfg.MaxWorkers
	if maxW <= 0 {
		maxW = DefaultMaxWorkers()
	}
	if maxW > len(jobs) {
		maxW = len(jobs)
	}
	sem := make(chan struct{}, maxW)
	g, ctx := errgroup.WithContext(ctx)
	var mu sync.Mutex
	var firstErr error

	for i, job := range jobs {
		i, job := i, job
		g.Go(func() error {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return ctx.Err()
			}
			defer func() { <-sem }()
			if err := job(ctx); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("job %d: %w", i, err)
				}
				mu.Unlock()
				return err
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return firstErr
	}
	return nil
}
