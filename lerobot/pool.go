package lerobot

import (
	"context"

	"github.com/ioai-tech/lerobot-go/internal/pool"
)

// RunEpisodeJobs executes staging jobs with bounded concurrency.
func RunEpisodeJobs(ctx context.Context, maxWorkers int, jobs []func(context.Context) error) error {
	wrapped := make([]pool.Job, len(jobs))
	for i, j := range jobs {
		j := j
		wrapped[i] = func(ctx context.Context) error { return j(ctx) }
	}
	return pool.Run(ctx, pool.Config{MaxWorkers: maxWorkers}, wrapped)
}
