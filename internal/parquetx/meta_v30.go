package parquetx

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/ioai-tech/lerobot-go/internal/meta"
	"github.com/ioai-tech/lerobot-go/internal/stats"
)

// WriteTasksParquet writes meta/tasks.parquet compatible with lerobot load_tasks().
// Task strings are stored as the pandas index column named "task".
func WriteTasksParquet(root string, taskMap map[string]int) error {
	if len(taskMap) == 0 {
		return nil
	}
	type row struct {
		task  string
		index int64
	}
	rows := make([]row, 0, len(taskMap))
	for task, idx := range taskMap {
		rows = append(rows, row{task: task, index: int64(idx)})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].index < rows[j].index })

	alloc := memory.NewGoAllocator()
	taskB := array.NewStringBuilder(alloc)
	idxB := array.NewInt64Builder(alloc)
	defer taskB.Release()
	defer idxB.Release()
	for _, r := range rows {
		taskB.Append(r.task)
		idxB.Append(r.index)
	}
	taskArr := taskB.NewArray()
	idxArr := idxB.NewArray()
	defer taskArr.Release()
	defer idxArr.Release()

	pandasMD := arrow.NewMetadata(
		[]string{"pandas"},
		[]string{`{"index_columns": ["task"], "column_indexes": [{"name": null, "field_name": null, "pandas_type": "unicode", "numpy_type": "object", "metadata": {"encoding": "UTF-8"}}], "columns": [{"name": "task_index", "field_name": "task_index", "pandas_type": "int64", "numpy_type": "int64", "metadata": null}, {"name": "task", "field_name": "task", "pandas_type": "unicode", "numpy_type": "object", "metadata": null}], "attributes": {}, "pandas_version": "2.3.3"}`},
	)
	schema := arrow.NewSchema([]arrow.Field{
		{Name: "task_index", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "task", Type: arrow.BinaryTypes.String, Nullable: true},
	}, &pandasMD)
	cols := []arrow.Column{
		*arrow.NewColumn(schema.Field(0), arrow.NewChunked(schema.Field(0).Type, []arrow.Array{idxArr})),
		*arrow.NewColumn(schema.Field(1), arrow.NewChunked(schema.Field(1).Type, []arrow.Array{taskArr})),
	}
	tbl := array.NewTable(schema, cols, int64(len(rows)))
	defer tbl.Release()

	path := filepath.Join(root, meta.DefaultTasksPath)
	return WriteTable(path, tbl, alloc)
}

type EpisodeMetaInput struct {
	EpisodeIndex int
	Tasks        []string
	Length       int
	Fields       map[string]any
	Stats        stats.EpisodeStats
	Features     map[string]meta.FeatureSpec
}

// WriteEpisodesParquet writes meta/episodes/*.parquet with chunk rotation by on-disk size.
func WriteEpisodesParquet(root string, rows []EpisodeMetaInput, fileSizeLimitMB int) error {
	if len(rows) == 0 {
		return nil
	}
	if fileSizeLimitMB <= 0 {
		fileSizeLimitMB = meta.DefaultDataFileSizeInMB
	}

	chunkIdx, fileIdx := 0, 0
	var buffer []EpisodeMetaInput
	var bufferEpisodes int

	flush := func() error {
		if len(buffer) == 0 {
			return nil
		}
		path := filepath.Join(root, meta.EpisodesMetaPath(chunkIdx, fileIdx))
		if err := writeEpisodesBatch(path, buffer); err != nil {
			return err
		}
		buffer = nil
		bufferEpisodes = 0
		return nil
	}

	for _, row := range rows {
		path := filepath.Join(root, meta.EpisodesMetaPath(chunkIdx, fileIdx))
		estSize := 0.0
		if bufferEpisodes > 0 {
			sizeMB, err := meta.FileSizeMB(path)
			if err == nil {
				estSize = sizeMB + sizeMB/float64(bufferEpisodes)*float64(row.Length)
			}
		}
		if bufferEpisodes > 0 && estSize >= float64(fileSizeLimitMB) {
			if err := flush(); err != nil {
				return err
			}
			chunkIdx, fileIdx = meta.UpdateChunkFileIndices(chunkIdx, fileIdx, meta.DefaultChunkSize)
		}
		row.Fields = copyEpisodeFields(row.Fields)
		row.Fields["meta/episodes/chunk_index"] = chunkIdx
		row.Fields["meta/episodes/file_index"] = fileIdx
		buffer = append(buffer, row)
		bufferEpisodes++
	}
	return flush()
}

type statColumnKind int

const (
	statColFloat64 statColumnKind = iota
	statColInt64
	statColImage311
)

func writeEpisodesBatch(path string, rows []EpisodeMetaInput) error {
	alloc := memory.NewGoAllocator()
	columnData := map[string]any{}
	columnTypes := map[string]arrow.DataType{}

	setInt64 := func(key string, v int64) {
		columnData[key] = appendInt64Slice(columnData[key], v)
		columnTypes[key] = arrow.PrimitiveTypes.Int64
	}
	setFloat64 := func(key string, v float64) {
		columnData[key] = appendFloat64Slice(columnData[key], v)
		columnTypes[key] = arrow.PrimitiveTypes.Float64
	}
	setFloat64List := func(key string, v []float64) {
		columnData[key] = appendFloat64ListSlice(columnData[key], v)
		columnTypes[key] = arrow.ListOf(arrow.PrimitiveTypes.Float64)
	}
	setInt64List := func(key string, v []int64) {
		columnData[key] = appendInt64ListSlice(columnData[key], v)
		columnTypes[key] = arrow.ListOf(arrow.PrimitiveTypes.Int64)
	}
	setImage311List := func(key string, v stats.ImageStat311) {
		columnData[key] = appendImage311ListSlice(columnData[key], v)
		inner := arrow.ListOf(arrow.ListOf(arrow.PrimitiveTypes.Float64))
		columnTypes[key] = arrow.ListOf(inner)
	}
	setStringList := func(key string, v []string) {
		columnData[key] = appendStringListSlice(columnData[key], v)
		columnTypes[key] = arrow.ListOf(arrow.BinaryTypes.String)
	}

	for _, row := range rows {
		setInt64("episode_index", int64(row.EpisodeIndex))
		setStringList("tasks", row.Tasks)
		setInt64("length", int64(row.Length))
		for key, val := range row.Fields {
			switch v := val.(type) {
			case int:
				setInt64(key, int64(v))
			case int64:
				setInt64(key, v)
			case float64:
				setFloat64(key, v)
			default:
				return fmt.Errorf("unsupported episode meta field %q type %T", key, val)
			}
		}
		for _, item := range flattenStats(row.Stats, row.Features) {
			switch item.kind {
			case statColInt64:
				setInt64List(item.key, item.val.([]int64))
			case statColImage311:
				setImage311List(item.key, item.val.(stats.ImageStat311))
			default:
				setFloat64List(item.key, item.val.([]float64))
			}
		}
	}

	keys := sortedStringKeys(columnData)
	fields := make([]arrow.Field, len(keys))
	arrays := make([]arrow.Array, len(keys))
	defer func() {
		for _, arr := range arrays {
			if arr != nil {
				arr.Release()
			}
		}
	}()

	for i, key := range keys {
		dt := columnTypes[key]
		fields[i] = arrow.Field{Name: key, Type: dt, Nullable: true}
		arr, err := buildMetaArray(alloc, dt, columnData[key], len(rows))
		if err != nil {
			return fmt.Errorf("column %q: %w", key, err)
		}
		arrays[i] = arr
	}

	schema := arrow.NewSchema(fields, nil)
	cols := make([]arrow.Column, len(fields))
	for i, field := range fields {
		chunked := arrow.NewChunked(field.Type, []arrow.Array{arrays[i]})
		cols[i] = *arrow.NewColumn(field, chunked)
		chunked.Release()
	}
	tbl := array.NewTable(schema, cols, int64(len(rows)))
	defer tbl.Release()
	return WriteTable(path, tbl, alloc)
}

func flattenStats(ep stats.EpisodeStats, features map[string]meta.FeatureSpec) []struct {
	key  string
	kind statColumnKind
	val  any
} {
	var out []struct {
		key  string
		kind statColumnKind
		val  any
	}
	for feature, fstats := range ep {
		spec := features[feature]
		imageLike := spec.DType == "image" || spec.DType == "video"
		for statKey, raw := range fstats {
			key := "stats/" + feature + "/" + statKey
			if statKey == "count" {
				out = append(out, struct {
					key  string
					kind statColumnKind
					val  any
				}{key, statColInt64, asInt64Slice(raw)})
				continue
			}
			if imageLike {
				if img, ok := raw.(stats.ImageStat311); ok {
					out = append(out, struct {
						key  string
						kind statColumnKind
						val  any
					}{key, statColImage311, img})
					continue
				}
			}
			out = append(out, struct {
				key  string
				kind statColumnKind
				val  any
			}{key, statColFloat64, asFloat64Slice(raw)})
		}
	}
	return out
}

func asFloat64Slice(v any) []float64 {
	switch x := v.(type) {
	case []float64:
		return x
	case stats.ImageStat311:
		return x.Channels()
	case []float32:
		out := make([]float64, len(x))
		for i, f := range x {
			out[i] = float64(f)
		}
		return out
	default:
		return nil
	}
}

func asInt64Slice(v any) []int64 {
	switch x := v.(type) {
	case []int64:
		return x
	case []float64:
		out := make([]int64, len(x))
		for i, f := range x {
			out[i] = int64(f)
		}
		return out
	default:
		return nil
	}
}

func buildMetaArray(alloc memory.Allocator, dt arrow.DataType, data any, nRows int) (arrow.Array, error) {
	switch dt.ID() {
	case arrow.INT64:
		vals := data.([]int64)
		b := array.NewInt64Builder(alloc)
		defer b.Release()
		for _, v := range vals {
			b.Append(v)
		}
		return b.NewArray(), nil
	case arrow.FLOAT64:
		vals := data.([]float64)
		b := array.NewFloat64Builder(alloc)
		defer b.Release()
		for _, v := range vals {
			b.Append(v)
		}
		return b.NewArray(), nil
	case arrow.LIST:
		switch v := data.(type) {
		case [][]float64:
			return buildListFloat64Array(alloc, v)
		case [][]int64:
			return buildListInt64Array(alloc, v)
		case []stats.ImageStat311:
			return buildListImage311Array(alloc, v)
		case [][]string:
			return buildListStringArray(alloc, v)
		default:
			return nil, fmt.Errorf("unsupported list data %T", data)
		}
	default:
		return nil, fmt.Errorf("unsupported arrow type %s", dt)
	}
}

func buildListInt64Array(alloc memory.Allocator, rows [][]int64) (arrow.Array, error) {
	lb := array.NewListBuilder(alloc, arrow.PrimitiveTypes.Int64)
	defer lb.Release()
	vb := lb.ValueBuilder().(*array.Int64Builder)
	for _, row := range rows {
		lb.Append(true)
		for _, v := range row {
			vb.Append(v)
		}
	}
	return lb.NewArray(), nil
}

func buildListImage311Array(alloc memory.Allocator, rows []stats.ImageStat311) (arrow.Array, error) {
	dt := arrow.ListOf(arrow.ListOf(arrow.ListOf(arrow.PrimitiveTypes.Float64)))
	lb := array.NewListBuilder(alloc, dt)
	defer lb.Release()
	l2b := lb.ValueBuilder().(*array.ListBuilder)
	l1b := l2b.ValueBuilder().(*array.ListBuilder)
	vb := l1b.ValueBuilder().(*array.Float64Builder)
	for _, row := range rows {
		lb.Append(true)
		for _, ch := range row {
			l2b.Append(true)
			for _, plane := range ch {
				l1b.Append(true)
				for _, v := range plane {
					vb.Append(v)
				}
			}
		}
	}
	return lb.NewArray(), nil
}

func buildListFloat64Array(alloc memory.Allocator, rows [][]float64) (arrow.Array, error) {
	lb := array.NewListBuilder(alloc, arrow.PrimitiveTypes.Float64)
	defer lb.Release()
	vb := lb.ValueBuilder().(*array.Float64Builder)
	for _, row := range rows {
		lb.Append(true)
		for _, v := range row {
			vb.Append(v)
		}
	}
	return lb.NewArray(), nil
}

func buildListStringArray(alloc memory.Allocator, rows [][]string) (arrow.Array, error) {
	lb := array.NewListBuilder(alloc, arrow.BinaryTypes.String)
	defer lb.Release()
	vb := lb.ValueBuilder().(*array.StringBuilder)
	for _, row := range rows {
		lb.Append(true)
		for _, v := range row {
			vb.Append(v)
		}
	}
	return lb.NewArray(), nil
}

func appendInt64Slice(cur any, v int64) []int64 {
	if cur == nil {
		return []int64{v}
	}
	return append(cur.([]int64), v)
}

func appendFloat64Slice(cur any, v float64) []float64 {
	if cur == nil {
		return []float64{v}
	}
	return append(cur.([]float64), v)
}

func appendFloat64ListSlice(cur any, v []float64) [][]float64 {
	if cur == nil {
		return [][]float64{v}
	}
	return append(cur.([][]float64), v)
}

func appendInt64ListSlice(cur any, v []int64) [][]int64 {
	if cur == nil {
		return [][]int64{v}
	}
	return append(cur.([][]int64), v)
}

func appendImage311ListSlice(cur any, v stats.ImageStat311) []stats.ImageStat311 {
	if cur == nil {
		return []stats.ImageStat311{v}
	}
	return append(cur.([]stats.ImageStat311), v)
}

func appendStringListSlice(cur any, v []string) [][]string {
	if cur == nil {
		return [][]string{v}
	}
	return append(cur.([][]string), v)
}

func sortedStringKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func copyEpisodeFields(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
