package parquetx

import (
	"context"
	"fmt"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
)

func ReadTable(ctx context.Context, path string, alloc memory.Allocator) (arrow.Table, error) {
	if alloc == nil {
		alloc = memory.NewGoAllocator()
	}
	pf, err := file.OpenParquetFile(path, true)
	if err != nil {
		return nil, err
	}
	defer func() { _ = pf.Close() }()

	reader, err := pqarrow.NewFileReader(pf, pqarrow.ArrowReadProperties{}, alloc)
	if err != nil {
		return nil, err
	}
	tbl, err := reader.ReadTable(ctx)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeImagePathColumns(tbl, alloc)
	if err != nil {
		tbl.Release()
		return nil, err
	}
	if normalized != tbl {
		tbl.Release()
	}
	return normalized, nil
}

func TableNumRows(path string) (int64, error) {
	pf, err := file.OpenParquetFile(path, true)
	if err != nil {
		return 0, err
	}
	defer func() { _ = pf.Close() }()
	return pf.NumRows(), nil
}

func ExtractInt64Column(tbl arrow.Table, name string) ([]int64, error) {
	idx := tbl.Schema().FieldIndices(name)
	if len(idx) == 0 {
		return nil, fmt.Errorf("column %q not found", name)
	}
	col := tbl.Column(idx[0])
	out := make([]int64, 0, col.Len())
	for _, chunk := range col.Data().Chunks() {
		arr, ok := chunk.(*array.Int64)
		if !ok {
			return nil, fmt.Errorf("column %q: expected int64", name)
		}
		for i := 0; i < arr.Len(); i++ {
			out = append(out, arr.Value(i))
		}
	}
	return out, nil
}

func ExtractFloat64Column(tbl arrow.Table, name string) ([]float64, error) {
	idx := tbl.Schema().FieldIndices(name)
	if len(idx) == 0 {
		return nil, fmt.Errorf("column %q not found", name)
	}
	col := tbl.Column(idx[0])
	out := make([]float64, 0, col.Len())
	for _, chunk := range col.Data().Chunks() {
		switch arr := chunk.(type) {
		case *array.Float64:
			for i := 0; i < arr.Len(); i++ {
				out = append(out, arr.Value(i))
			}
		case *array.Float32:
			for i := 0; i < arr.Len(); i++ {
				out = append(out, float64(arr.Value(i)))
			}
		default:
			return nil, fmt.Errorf("column %q: expected float", name)
		}
	}
	return out, nil
}

func normalizeImagePathColumns(tbl arrow.Table, alloc memory.Allocator) (arrow.Table, error) {
	schema := tbl.Schema()
	needsNormalize := false
	fields := schema.Fields()
	for _, field := range fields {
		st, ok := field.Type.(*arrow.StructType)
		if !ok || st.NumFields() != 2 {
			continue
		}
		if st.Field(0).Name == "bytes" && st.Field(1).Name == "path" && st.Field(1).Type.ID() == arrow.NULL {
			needsNormalize = true
			break
		}
	}
	if !needsNormalize {
		return tbl, nil
	}

	newFields := make([]arrow.Field, len(fields))
	copy(newFields, fields)
	newCols := make([]arrow.Column, int(tbl.NumCols()))
	for i := 0; i < int(tbl.NumCols()); i++ {
		field := fields[i]
		st, ok := field.Type.(*arrow.StructType)
		if !ok || st.NumFields() != 2 || st.Field(0).Name != "bytes" || st.Field(1).Name != "path" || st.Field(1).Type.ID() != arrow.NULL {
			col := tbl.Column(i)
			col.Retain()
			newCols[i] = *col
			continue
		}
		newField := arrow.Field{Name: field.Name, Type: ImageArrowType(), Nullable: field.Nullable, Metadata: field.Metadata}
		newFields[i] = newField
		var chunks []arrow.Array
		for _, chunk := range tbl.Column(i).Data().Chunks() {
			arr, ok := chunk.(*array.Struct)
			if !ok {
				return nil, fmt.Errorf("column %q: expected struct image array", field.Name)
			}
			normalized, err := normalizeImageChunk(arr, alloc)
			if err != nil {
				return nil, err
			}
			chunks = append(chunks, normalized)
		}
		chunked := arrow.NewChunked(newField.Type, chunks)
		newCols[i] = *arrow.NewColumn(newField, chunked)
		chunked.Release()
		for _, chunk := range chunks {
			chunk.Release()
		}
	}
	md := schema.Metadata()
	newSchema := arrow.NewSchema(newFields, &md)
	return array.NewTable(newSchema, newCols, tbl.NumRows()), nil
}

func normalizeImageChunk(arr *array.Struct, alloc memory.Allocator) (arrow.Array, error) {
	sb := array.NewStructBuilder(alloc, ImageArrowType().(*arrow.StructType))
	defer sb.Release()
	bytesB := sb.FieldBuilder(0).(*array.BinaryBuilder)
	pathB := sb.FieldBuilder(1).(*array.StringBuilder)
	bytesArr, ok := arr.Field(0).(*array.Binary)
	if !ok {
		return nil, fmt.Errorf("image bytes field must be binary")
	}
	for i := 0; i < arr.Len(); i++ {
		if arr.IsNull(i) {
			sb.AppendNull()
			continue
		}
		sb.Append(true)
		bytesB.Append(bytesArr.Value(i))
		pathB.AppendNull()
	}
	return sb.NewArray(), nil
}
