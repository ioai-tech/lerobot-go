package parquetx

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/ioai-tech/lerobot-go/internal/meta"
)

type EpisodeMetaRow struct {
	EpisodeIndex     int64
	Length           int64
	Tasks            []string
	DatasetFromIndex int64
	DatasetToIndex   int64
	DataChunkIndex   int64
	DataFileIndex    int64
	VideoFields      map[string]any
	StatsFields      map[string][]float64
}

func ReadEpisodesMeta(root string) ([]EpisodeMetaRow, error) {
	glob := filepath.Join(root, meta.EpisodesDir, "chunk-*", "file-*.parquet")
	paths, err := filepath.Glob(glob)
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	ctx := context.Background()
	var all []EpisodeMetaRow
	for _, p := range paths {
		rows, err := readEpisodesFile(ctx, p)
		if err != nil {
			return nil, err
		}
		all = append(all, rows...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].EpisodeIndex < all[j].EpisodeIndex })
	return all, nil
}

func readEpisodesFile(ctx context.Context, path string) ([]EpisodeMetaRow, error) {
	tbl, err := ReadTable(ctx, path, memory.NewGoAllocator())
	if err != nil {
		return nil, err
	}
	defer tbl.Release()
	n := int(tbl.NumRows())
	epIdx, err := ExtractInt64Column(tbl, "episode_index")
	if err != nil {
		return nil, err
	}
	lengths, err := ExtractInt64Column(tbl, "length")
	if err != nil {
		return nil, err
	}
	fromIdx, _ := ExtractInt64Column(tbl, "dataset_from_index")
	toIdx, _ := ExtractInt64Column(tbl, "dataset_to_index")
	dataChunk, _ := ExtractInt64Column(tbl, "data/chunk_index")
	dataFile, _ := ExtractInt64Column(tbl, "data/file_index")

	rows := make([]EpisodeMetaRow, n)
	for i := 0; i < n; i++ {
		rows[i] = EpisodeMetaRow{
			EpisodeIndex:     epIdx[i],
			Length:           lengths[i],
			DatasetFromIndex: pick(fromIdx, i),
			DatasetToIndex:   pick(toIdx, i),
			DataChunkIndex:   pick(dataChunk, i),
			DataFileIndex:    pick(dataFile, i),
			VideoFields:      map[string]any{},
		}
		if tasksCol, ok := findListStringColumn(tbl, "tasks"); ok {
			rows[i].Tasks = tasksCol[i]
		}
	}
	for ci := 0; ci < int(tbl.NumCols()); ci++ {
		name := tbl.Schema().Field(ci).Name
		switch {
		case strings.HasPrefix(name, "videos/"):
			if vals, err := extractNumericListColumn(tbl, name); err == nil && len(vals) == n {
				for i := 0; i < n; i++ {
					rows[i].VideoFields[name] = vals[i]
				}
			} else if scalars, err := ExtractInt64Column(tbl, name); err == nil && len(scalars) == n {
				for i := 0; i < n; i++ {
					rows[i].VideoFields[name] = []float64{float64(scalars[i])}
				}
			} else if scalars, err := ExtractFloat64Column(tbl, name); err == nil && len(scalars) == n {
				for i := 0; i < n; i++ {
					rows[i].VideoFields[name] = []float64{scalars[i]}
				}
			}
		case strings.HasPrefix(name, "stats/"):
			if vals, err := extractNumericListColumn(tbl, name); err == nil && len(vals) == n {
				for i := 0; i < n; i++ {
					if rows[i].StatsFields == nil {
						rows[i].StatsFields = map[string][]float64{}
					}
					rows[i].StatsFields[name] = vals[i]
				}
			}
		}
	}
	return rows, nil
}

// UnflattenEpisodeStats converts stats/feature/key parquet columns to v2.1 jsonl shape.
func UnflattenEpisodeStats(fields map[string][]float64) map[string]any {
	out := map[string]any{}
	for key, vals := range fields {
		parts := strings.SplitN(key, "/", 3)
		if len(parts) != 3 {
			continue
		}
		feature := parts[1]
		statKey := parts[2]
		feat, ok := out[feature].(map[string]any)
		if !ok {
			feat = map[string]any{}
			out[feature] = feat
		}
		feat[statKey] = vals
	}
	return out
}

func pick(vals []int64, i int) int64 {
	if i < len(vals) {
		return vals[i]
	}
	return 0
}

func findListStringColumn(tbl arrow.Table, name string) ([][]string, bool) {
	idx := tbl.Schema().FieldIndices(name)
	if len(idx) == 0 {
		return nil, false
	}
	col := tbl.Column(idx[0])
	var out [][]string
	for _, chunk := range col.Data().Chunks() {
		lst, ok := chunk.(*array.List)
		if !ok {
			return nil, false
		}
		vb := lst.ListValues().(*array.String)
		offsets := lst.Offsets()
		for i := 0; i < lst.Len(); i++ {
			start, end := offsets[i], offsets[i+1]
			row := make([]string, end-start)
			for j := start; j < end; j++ {
				row[j-start] = vb.Value(int(j))
			}
			out = append(out, row)
		}
	}
	return out, true
}

func extractNumericListColumn(tbl arrow.Table, name string) ([][]float64, error) {
	idx := tbl.Schema().FieldIndices(name)
	if len(idx) == 0 {
		return nil, fmt.Errorf("missing %s", name)
	}
	col := tbl.Column(idx[0])
	var out [][]float64
	for _, chunk := range col.Data().Chunks() {
		lst, ok := chunk.(*array.List)
		if !ok {
			return nil, fmt.Errorf("%s: expected list", name)
		}
		offsets := lst.Offsets()
		switch values := lst.ListValues().(type) {
		case *array.Float64:
			for i := 0; i < lst.Len(); i++ {
				start, end := offsets[i], offsets[i+1]
				row := make([]float64, end-start)
				for j := start; j < end; j++ {
					row[j-start] = values.Value(int(j))
				}
				out = append(out, row)
			}
		case *array.Int64:
			for i := 0; i < lst.Len(); i++ {
				start, end := offsets[i], offsets[i+1]
				row := make([]float64, end-start)
				for j := start; j < end; j++ {
					row[j-start] = float64(values.Value(int(j)))
				}
				out = append(out, row)
			}
		default:
			return nil, fmt.Errorf("%s: unsupported list value type", name)
		}
	}
	return out, nil
}
