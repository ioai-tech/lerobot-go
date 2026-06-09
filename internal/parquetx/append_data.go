package parquetx

import (
	"context"
	"fmt"
	"os"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

type AppendEpisodeOptions struct {
	GlobalFrameIndex int64
	EpisodeIndex     int64
	EpisodeTasks     []string
	GlobalTaskIndex  map[string]int
}

type EpisodeBatchEntry struct {
	SourcePath string
	Options    AppendEpisodeOptions
}

func AppendEpisodeParquet(ctx context.Context, dst, src string, opts AppendEpisodeOptions) error {
	alloc := memory.NewGoAllocator()
	rewritten, err := RewriteEpisodeParquet(ctx, src, opts, alloc)
	if err != nil {
		return err
	}
	defer rewritten.Release()

	if _, err := os.Stat(dst); os.IsNotExist(err) {
		return WriteTable(dst, rewritten, alloc)
	}
	existing, err := ReadTable(ctx, dst, alloc)
	if err != nil {
		return err
	}
	defer existing.Release()

	merged, err := ConcatTables(alloc, []arrow.Table{existing, rewritten})
	if err != nil {
		return err
	}
	defer merged.Release()
	return WriteTable(dst, merged, alloc)
}

func WriteEpisodeBatch(ctx context.Context, dst string, entries []EpisodeBatchEntry) error {
	if len(entries) == 0 {
		return fmt.Errorf("no episode parquet entries")
	}
	alloc := memory.NewGoAllocator()
	first, err := RewriteEpisodeParquet(ctx, entries[0].SourcePath, entries[0].Options, alloc)
	if err != nil {
		return err
	}
	defer first.Release()
	writer, err := NewAppendWriter(dst, first.Schema())
	if err != nil {
		return err
	}
	if err := writer.WriteTable(first, 1024); err != nil {
		_ = writer.Close()
		return err
	}
	for _, entry := range entries[1:] {
		tbl, err := RewriteEpisodeParquet(ctx, entry.SourcePath, entry.Options, alloc)
		if err != nil {
			_ = writer.Close()
			return err
		}
		if err := writer.WriteTable(tbl, 1024); err != nil {
			tbl.Release()
			_ = writer.Close()
			return err
		}
		tbl.Release()
	}
	return writer.Close()
}

func RewriteEpisodeParquet(ctx context.Context, src string, opts AppendEpisodeOptions, alloc memory.Allocator) (arrow.Table, error) {
	tbl, err := ReadTable(ctx, src, alloc)
	if err != nil {
		return nil, err
	}
	defer tbl.Release()

	n := int(tbl.NumRows())
	indices := make([]int64, n)
	epIndices := make([]int64, n)
	for i := 0; i < n; i++ {
		indices[i] = opts.GlobalFrameIndex + int64(i)
		epIndices[i] = opts.EpisodeIndex
	}

	step1, err := ReplaceInt64Column(tbl, "index", indices, alloc)
	if err != nil {
		return nil, err
	}
	step2, err := ReplaceInt64Column(step1, "episode_index", epIndices, alloc)
	step1.Release()
	if err != nil {
		return nil, err
	}

	// Staging parquet uses per-episode local task_index (0..n-1). v2.1 on-disk parquet
	// after merge already stores dataset-global task_index values — register both keys.
	localToGlobal := make(map[int64]int64, len(opts.EpisodeTasks)*2)
	for localIdx, task := range opts.EpisodeTasks {
		globalIdx, ok := opts.GlobalTaskIndex[task]
		if !ok {
			return nil, fmt.Errorf("task %q missing from global task map", task)
		}
		g := int64(globalIdx)
		localToGlobal[int64(localIdx)] = g
		localToGlobal[g] = g
	}
	localTaskIdx, err := ExtractInt64Column(step2, "task_index")
	if err != nil {
		step2.Release()
		return nil, err
	}
	taskIndices := make([]int64, len(localTaskIdx))
	for i, local := range localTaskIdx {
		global, ok := localToGlobal[local]
		if !ok {
			step2.Release()
			return nil, fmt.Errorf("local task_index %d not mapped", local)
		}
		taskIndices[i] = global
	}
	final, err := ReplaceInt64Column(step2, "task_index", taskIndices, alloc)
	step2.Release()
	if err != nil {
		return nil, err
	}
	final.Retain()
	return final, nil
}
