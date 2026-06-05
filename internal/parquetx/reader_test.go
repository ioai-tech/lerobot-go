package parquetx

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

func TestReadTableNormalizesNullImagePathFields(t *testing.T) {
	alloc := memory.NewGoAllocator()
	imageType := arrow.StructOf(
		arrow.Field{Name: "bytes", Type: arrow.BinaryTypes.Binary, Nullable: true},
		arrow.Field{Name: "path", Type: arrow.Null, Nullable: true},
	)
	sb := array.NewStructBuilder(alloc, imageType)
	defer sb.Release()
	bytesB := sb.FieldBuilder(0).(*array.BinaryBuilder)
	nullB := sb.FieldBuilder(1).(*array.NullBuilder)
	sb.Append(true)
	bytesB.Append([]byte("a"))
	nullB.AppendNull()
	arr := sb.NewArray()
	defer arr.Release()

	schema := arrow.NewSchema([]arrow.Field{{Name: "observation.image", Type: imageType, Nullable: true}}, nil)
	tbl := array.NewTable(schema, []arrow.Column{
		*arrow.NewColumn(schema.Field(0), arrow.NewChunked(schema.Field(0).Type, []arrow.Array{arr})),
	}, 1)
	defer tbl.Release()

	path := filepath.Join(t.TempDir(), "image-null-path.parquet")
	if err := WriteTable(path, tbl, alloc); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadTable(context.Background(), path, alloc)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Release()

	field := loaded.Schema().Field(0)
	if field.Type.String() != ImageArrowType().String() {
		t.Fatalf("normalized type=%s want %s", field.Type, ImageArrowType())
	}
}
