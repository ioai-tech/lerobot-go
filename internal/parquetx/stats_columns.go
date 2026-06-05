package parquetx

import (
	"context"
	"fmt"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
)

// ColumnsForStats extracts numeric columns from an episode parquet for stats computation.
func ColumnsForStats(ctx context.Context, path string) (map[string]any, error) {
	tbl, err := ReadTable(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	defer tbl.Release()

	out := make(map[string]any, int(tbl.NumCols()))
	for i := 0; i < int(tbl.NumCols()); i++ {
		field := tbl.Schema().Field(i)
		if field.Type.ID() == arrow.STRUCT {
			continue
		}
		vals, err := columnToStatsValues(tbl.Column(i), field.Type)
		if err != nil {
			return nil, err
		}
		if vals != nil {
			out[field.Name] = vals
		}
	}
	return out, nil
}

func columnToStatsValues(col *arrow.Column, dt arrow.DataType) (any, error) {
	switch dt.ID() {
	case arrow.INT64:
		return extractInt64Col(col)
	case arrow.FLOAT64:
		return extractFloat64Col(col)
	case arrow.FLOAT32:
		vals, err := extractFloat64Col(col)
		if err != nil {
			return nil, err
		}
		out := make([]float32, len(vals))
		for i, v := range vals {
			out[i] = float32(v)
		}
		return out, nil
	case arrow.FIXED_SIZE_LIST:
		fsl := dt.(*arrow.FixedSizeListType)
		if _, ok := fsl.Elem().(*arrow.Float32Type); ok {
			return extractFixedSizeListFloat(col, fsl)
		}
		if _, ok := fsl.Elem().(*arrow.Float64Type); ok {
			return extractFixedSizeListFloat64(col, int(fsl.Len()))
		}
		return nil, nil
	default:
		return nil, nil
	}
}

func extractInt64Col(col *arrow.Column) ([]int64, error) {
	out := make([]int64, 0, col.Len())
	for _, chunk := range col.Data().Chunks() {
		arr, ok := chunk.(*array.Int64)
		if !ok {
			return nil, errUnsupportedColType
		}
		for i := 0; i < arr.Len(); i++ {
			out = append(out, arr.Value(i))
		}
	}
	return out, nil
}

func extractFloat64Col(col *arrow.Column) ([]float64, error) {
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
			return nil, errUnsupportedColType
		}
	}
	return out, nil
}

func extractFixedSizeListFloat(col *arrow.Column, dt *arrow.FixedSizeListType) ([][]float32, error) {
	size := int(dt.Len())
	out := make([][]float32, 0, col.Len())
	for _, chunk := range col.Data().Chunks() {
		arr, ok := chunk.(*array.FixedSizeList)
		if !ok {
			return nil, errUnsupportedColType
		}
		vb := arr.ListValues().(*array.Float32)
		for i := 0; i < arr.Len(); i++ {
			if arr.IsNull(i) {
				continue
			}
			start := i * size
			row := make([]float32, size)
			for j := 0; j < size; j++ {
				row[j] = vb.Value(start + j)
			}
			out = append(out, row)
		}
	}
	return out, nil
}

func extractFixedSizeListFloat64(col *arrow.Column, size int) ([][]float64, error) {
	out := make([][]float64, 0, col.Len())
	for _, chunk := range col.Data().Chunks() {
		arr, ok := chunk.(*array.FixedSizeList)
		if !ok {
			return nil, errUnsupportedColType
		}
		vb := arr.ListValues().(*array.Float64)
		for i := 0; i < arr.Len(); i++ {
			if arr.IsNull(i) {
				continue
			}
			start := i * size
			row := make([]float64, size)
			for j := 0; j < size; j++ {
				row[j] = vb.Value(start + j)
			}
			out = append(out, row)
		}
	}
	return out, nil
}

var errUnsupportedColType = fmt.Errorf("unsupported column type for stats")
