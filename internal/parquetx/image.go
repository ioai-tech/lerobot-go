package parquetx

import (
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

// ImageCell matches HuggingFace datasets.Image embedded parquet storage.
type ImageCell struct {
	Bytes []byte
	Path  string
}

func ImageArrowType() arrow.DataType {
	return arrow.StructOf(
		arrow.Field{Name: "bytes", Type: arrow.BinaryTypes.Binary, Nullable: true},
		arrow.Field{Name: "path", Type: arrow.BinaryTypes.String, Nullable: true},
	)
}

func buildImageArray(alloc memory.Allocator, cells []ImageCell) (arrow.Array, error) {
	sb := array.NewStructBuilder(alloc, ImageArrowType().(*arrow.StructType))
	defer sb.Release()
	bytesB := sb.FieldBuilder(0).(*array.BinaryBuilder)
	pathB := sb.FieldBuilder(1).(*array.StringBuilder)
	for _, cell := range cells {
		sb.Append(true)
		bytesB.Append(cell.Bytes)
		pathB.Append(cell.Path)
	}
	return sb.NewArray(), nil
}
