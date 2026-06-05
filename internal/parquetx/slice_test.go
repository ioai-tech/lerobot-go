package parquetx

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

func TestSliceTableByIndexRange(t *testing.T) {
	alloc := memory.NewGoAllocator()
	indexBuilder := array.NewInt64Builder(alloc)
	valueBuilder := array.NewFloat32Builder(alloc)
	defer indexBuilder.Release()
	defer valueBuilder.Release()
	for i := int64(10); i < 20; i++ {
		indexBuilder.Append(i)
		valueBuilder.Append(float32(i))
	}
	indexArr := indexBuilder.NewArray()
	valueArr := valueBuilder.NewArray()
	defer indexArr.Release()
	defer valueArr.Release()

	schema := arrow.NewSchema([]arrow.Field{
		{Name: "index", Type: arrow.PrimitiveTypes.Int64},
		{Name: "value", Type: arrow.PrimitiveTypes.Float32},
	}, nil)
	tbl := array.NewTable(schema, []arrow.Column{
		*arrow.NewColumn(schema.Field(0), arrow.NewChunked(schema.Field(0).Type, []arrow.Array{indexArr})),
		*arrow.NewColumn(schema.Field(1), arrow.NewChunked(schema.Field(1).Type, []arrow.Array{valueArr})),
	}, 10)
	defer tbl.Release()

	slice, err := SliceTableByIndexRange(tbl, 12, 15)
	if err != nil {
		t.Fatal(err)
	}
	defer slice.Release()
	if got := slice.NumRows(); got != 3 {
		t.Fatalf("rows=%d want 3", got)
	}
	idx, err := ExtractInt64Column(slice, "index")
	if err != nil {
		t.Fatal(err)
	}
	if idx[0] != 12 || idx[2] != 14 {
		t.Fatalf("index slice=%v", idx)
	}
}

func TestSliceTableByIndexRangeOnOfficialPushTImage(t *testing.T) {
	root := "/workspace/.tmp/official-datasets/lerobot-pusht-image"
	path := filepath.Join(root, "data", "chunk-000", "file-001.parquet")
	if _, err := TableNumRows(path); err != nil {
		t.Skip("official lerobot/pusht_image dataset not downloaded")
	}
	tbl, err := ReadTable(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tbl.Release()
	slice, err := SliceTableByIndexRange(tbl, 2964, 3125)
	if err != nil {
		t.Fatal(err)
	}
	defer slice.Release()
	if got := slice.NumRows(); got != 161 {
		t.Fatalf("rows=%d want 161", got)
	}
	idx, err := ExtractInt64Column(slice, "index")
	if err != nil {
		t.Fatal(err)
	}
	if idx[0] != 2964 || idx[len(idx)-1] != 3124 {
		t.Fatalf("index slice=%v...%v", idx[0], idx[len(idx)-1])
	}
}
