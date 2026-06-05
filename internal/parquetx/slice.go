package parquetx

import (
	"context"
	"fmt"
	"sort"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

func SliceTable(tbl arrow.Table, offset, length int64) (arrow.Table, error) {
	if length < 0 || offset < 0 || offset+length > tbl.NumRows() {
		return nil, fmt.Errorf("invalid slice offset=%d length=%d rows=%d", offset, length, tbl.NumRows())
	}
	schema := tbl.Schema()
	cols := make([]arrow.Column, int(tbl.NumCols()))
	for i := 0; i < int(tbl.NumCols()); i++ {
		col := tbl.Column(i)
		var slices []arrow.Array
		remaining := length
		pos := offset
		for _, chunk := range col.Data().Chunks() {
			chunkLen := int64(chunk.Len())
			if pos >= chunkLen {
				pos -= chunkLen
				continue
			}
			take := remaining
			if take > chunkLen-pos {
				take = chunkLen - pos
			}
			slices = append(slices, array.NewSlice(chunk, pos, pos+take))
			remaining -= take
			pos = 0
			if remaining == 0 {
				break
			}
		}
		if remaining != 0 {
			return nil, fmt.Errorf("slice failed for column %q", schema.Field(i).Name)
		}
		chunked := arrow.NewChunked(schema.Field(i).Type, slices)
		cols[i] = *arrow.NewColumn(schema.Field(i), chunked)
		chunked.Release()
	}
	return array.NewTable(schema, cols, length), nil
}

func SliceTableByIndexRange(tbl arrow.Table, fromIndex, toIndex int64) (arrow.Table, error) {
	if toIndex < fromIndex {
		return nil, fmt.Errorf("invalid index range from=%d to=%d", fromIndex, toIndex)
	}
	indices, err := ExtractInt64Column(tbl, "index")
	if err != nil {
		return nil, err
	}
	start := -1
	end := -1
	for i, idx := range indices {
		if start < 0 && idx >= fromIndex {
			start = i
		}
		if idx < toIndex {
			end = i + 1
		}
	}
	if start < 0 || end < start {
		return nil, fmt.Errorf("index range %d:%d not found", fromIndex, toIndex)
	}
	if end-start != int(toIndex-fromIndex) {
		return nil, fmt.Errorf("index range %d:%d length=%d want %d", fromIndex, toIndex, end-start, toIndex-fromIndex)
	}
	return SliceTable(tbl, int64(start), int64(end-start))
}

func LoadAllDataTables(ctx context.Context, paths []string, alloc memory.Allocator) (arrow.Table, error) {
	if alloc == nil {
		alloc = memory.NewGoAllocator()
	}
	sort.Strings(paths)
	var tables []arrow.Table
	defer func() {
		for _, t := range tables {
			t.Release()
		}
	}()
	for _, p := range paths {
		t, err := ReadTable(ctx, p, alloc)
		if err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	if len(tables) == 0 {
		return nil, fmt.Errorf("no parquet files")
	}
	if len(tables) == 1 {
		tables[0].Retain()
		return tables[0], nil
	}
	merged, err := ConcatTables(alloc, tables)
	if err != nil {
		return nil, err
	}
	merged.Retain()
	return merged, nil
}
