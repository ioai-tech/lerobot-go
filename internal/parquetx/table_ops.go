package parquetx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
)

func WriteTable(path string, tbl arrow.Table, alloc memory.Allocator) error {
	if alloc == nil {
		alloc = memory.NewGoAllocator()
	}
	_ = alloc
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	props := parquet.NewWriterProperties(
		parquet.WithCompression(compress.Codecs.Snappy),
		parquet.WithDictionaryDefault(true),
	)
	defer func() { _ = f.Close() }()
	return pqarrow.WriteTable(tbl, f, 1024, props, pqarrow.DefaultWriterProps())
}

func ConcatTables(alloc memory.Allocator, tables []arrow.Table) (arrow.Table, error) {
	if len(tables) == 0 {
		return nil, fmt.Errorf("no tables to concatenate")
	}
	if len(tables) == 1 {
		tables[0].Retain()
		return tables[0], nil
	}
	if alloc == nil {
		alloc = memory.NewGoAllocator()
	}
	schema := tables[0].Schema()
	nCols := int(tables[0].NumCols())
	var totalRows int64
	for _, tbl := range tables {
		if !schemasCompatible(schema, tbl.Schema()) {
			return nil, fmt.Errorf("schema mismatch during concat")
		}
		totalRows += tbl.NumRows()
	}

	cols := make([]arrow.Column, nCols)
	for c := 0; c < nCols; c++ {
		field := schema.Field(c)
		var chunks []arrow.Array
		for _, tbl := range tables {
			col := tbl.Column(c)
			chunks = append(chunks, col.Data().Chunks()...)
		}
		combined, err := array.Concatenate(chunks, alloc)
		if err != nil {
			for _, col := range cols[:c] {
				col.Release()
			}
			return nil, err
		}
		chunked := arrow.NewChunked(field.Type, []arrow.Array{combined})
		cols[c] = *arrow.NewColumn(field, chunked)
		chunked.Release()
		combined.Release()
	}
	return array.NewTable(schema, cols, totalRows), nil
}

func ReplaceInt64Column(tbl arrow.Table, name string, values []int64, alloc memory.Allocator) (arrow.Table, error) {
	if alloc == nil {
		alloc = memory.NewGoAllocator()
	}
	if int64(len(values)) != tbl.NumRows() {
		return nil, fmt.Errorf("column %q: expected %d values, got %d", name, tbl.NumRows(), len(values))
	}
	idx := tbl.Schema().FieldIndices(name)
	if len(idx) == 0 {
		return nil, fmt.Errorf("column %q not found", name)
	}
	colIdx := idx[0]

	b := array.NewInt64Builder(alloc)
	defer b.Release()
	for _, v := range values {
		b.Append(v)
	}
	newArr := b.NewArray()
	defer newArr.Release()

	schema := tbl.Schema()
	cols := make([]arrow.Column, int(tbl.NumCols()))
	for i := 0; i < len(cols); i++ {
		if i == colIdx {
			chunked := arrow.NewChunked(schema.Field(i).Type, []arrow.Array{newArr})
			cols[i] = *arrow.NewColumn(schema.Field(i), chunked)
			chunked.Release()
			continue
		}
		src := tbl.Column(i)
		src.Retain()
		cols[i] = *src
	}
	return array.NewTable(schema, cols, tbl.NumRows()), nil
}

func MergeParquetFiles(ctx context.Context, dst string, sources []string, alloc memory.Allocator) error {
	if len(sources) == 0 {
		return fmt.Errorf("no source parquet files")
	}
	if alloc == nil {
		alloc = memory.NewGoAllocator()
	}
	tables := make([]arrow.Table, 0, len(sources))
	defer func() {
		for _, t := range tables {
			t.Release()
		}
	}()
	for _, src := range sources {
		tbl, err := ReadTable(ctx, src, alloc)
		if err != nil {
			return err
		}
		tables = append(tables, tbl)
	}
	merged, err := ConcatTables(alloc, tables)
	if err != nil {
		return err
	}
	defer merged.Release()
	return WriteTable(dst, merged, alloc)
}

func schemasCompatible(a, b *arrow.Schema) bool {
	if a == nil || b == nil {
		return a == b
	}
	if len(a.Fields()) != len(b.Fields()) {
		return false
	}
	for i, af := range a.Fields() {
		bf := b.Field(i)
		if af.Name != bf.Name || af.Nullable != bf.Nullable || af.Type.String() != bf.Type.String() {
			return false
		}
	}
	return true
}
