package parquetx

import (
	"context"
	"fmt"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

// ReadTasksParquet returns task string -> task_index.
func ReadTasksParquet(ctx context.Context, path string) (map[string]int, error) {
	tbl, err := ReadTable(ctx, path, memory.NewGoAllocator())
	if err != nil {
		return nil, err
	}
	defer tbl.Release()
	tasks, err := columnStrings(tbl, "task")
	if err != nil {
		tasks, err = columnStrings(tbl, "__index_level_0__")
		if err != nil {
			return nil, err
		}
	}
	indices, err := ExtractInt64Column(tbl, "task_index")
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(tasks))
	for i, t := range tasks {
		out[t] = int(indices[i])
	}
	return out, nil
}

func columnStrings(tbl arrow.Table, name string) ([]string, error) {
	idx := tbl.Schema().FieldIndices(name)
	if len(idx) == 0 {
		return nil, fmt.Errorf("column %q not found", name)
	}
	col := tbl.Column(idx[0])
	var out []string
	for _, chunk := range col.Data().Chunks() {
		arr, ok := chunk.(*array.String)
		if !ok {
			return nil, fmt.Errorf("column %q: expected string", name)
		}
		for i := 0; i < arr.Len(); i++ {
			out = append(out, arr.Value(i))
		}
	}
	return out, nil
}
