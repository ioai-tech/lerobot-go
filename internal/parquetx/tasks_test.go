package parquetx

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/ioai-tech/lerobot-go/internal/meta"
)

func TestWriteTasksParquetUsesTaskColumn(t *testing.T) {
	root := t.TempDir()
	taskMap := map[string]int{
		"pick":  0,
		"place": 1,
	}
	if err := WriteTasksParquet(root, taskMap); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, meta.DefaultTasksPath)
	tbl, err := ReadTable(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()

	if len(tbl.Schema().FieldIndices("task")) == 0 {
		t.Fatal("task column missing")
	}
	if len(tbl.Schema().FieldIndices("__index_level_0__")) != 0 {
		t.Fatal("legacy __index_level_0__ column should not be written for new datasets")
	}

	loaded, err := ReadTasksParquet(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded["pick"] != 0 || loaded["place"] != 1 {
		t.Fatalf("loaded task map=%v", loaded)
	}
}

func TestConcatTablesIgnoresSchemaMetadataDifferences(t *testing.T) {
	alloc := memory.NewGoAllocator()
	builderA := array.NewInt64Builder(alloc)
	builderB := array.NewInt64Builder(alloc)
	defer builderA.Release()
	defer builderB.Release()
	builderA.AppendValues([]int64{1, 2}, nil)
	builderB.AppendValues([]int64{3, 4}, nil)
	arrA := builderA.NewArray()
	arrB := builderB.NewArray()
	defer arrA.Release()
	defer arrB.Release()

	mdA := arrow.NewMetadata([]string{"huggingface"}, []string{"{\"a\":1}"})
	mdB := arrow.NewMetadata([]string{"huggingface"}, []string{"{\"a\":2}"})
	schemaA := arrow.NewSchema([]arrow.Field{{Name: "index", Type: arrow.PrimitiveTypes.Int64}}, &mdA)
	schemaB := arrow.NewSchema([]arrow.Field{{Name: "index", Type: arrow.PrimitiveTypes.Int64}}, &mdB)
	tblA := array.NewTable(schemaA, []arrow.Column{
		*arrow.NewColumn(schemaA.Field(0), arrow.NewChunked(schemaA.Field(0).Type, []arrow.Array{arrA})),
	}, 2)
	tblB := array.NewTable(schemaB, []arrow.Column{
		*arrow.NewColumn(schemaB.Field(0), arrow.NewChunked(schemaB.Field(0).Type, []arrow.Array{arrB})),
	}, 2)
	defer tblA.Release()
	defer tblB.Release()

	merged, err := ConcatTables(alloc, []arrow.Table{tblA, tblB})
	if err != nil {
		t.Fatal(err)
	}
	defer merged.Release()
	if merged.NumRows() != 4 {
		t.Fatalf("rows=%d", merged.NumRows())
	}
}
