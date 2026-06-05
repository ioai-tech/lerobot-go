package parquetx

import (
	"fmt"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/ioai-tech/lerobot-go/internal/meta"
)

func BuildArrowSchema(features map[string]meta.FeatureSpec) (*arrow.Schema, error) {
	fields := make([]arrow.Field, 0, len(features))
	names := sortedKeys(features)
	for _, name := range names {
		spec := features[name]
		if spec.DType == "video" {
			continue
		}
		if spec.DType == "image" {
			fields = append(fields, arrow.Field{Name: name, Type: ImageArrowType(), Nullable: true})
			continue
		}
		dt, err := arrowTypeForFeature(spec)
		if err != nil {
			return nil, fmt.Errorf("feature %q: %w", name, err)
		}
		fields = append(fields, arrow.Field{Name: name, Type: dt, Nullable: true})
	}
	return arrow.NewSchema(fields, nil), nil
}

func arrowTypeForFeature(spec meta.FeatureSpec) (arrow.DataType, error) {
	switch spec.DType {
	case "float32":
		return vectorType(arrow.PrimitiveTypes.Float32, spec.Shape), nil
	case "float64":
		return vectorType(arrow.PrimitiveTypes.Float64, spec.Shape), nil
	case "int64":
		return vectorType(arrow.PrimitiveTypes.Int64, spec.Shape), nil
	case "int32":
		return vectorType(arrow.PrimitiveTypes.Int32, spec.Shape), nil
	case "bool":
		return vectorType(arrow.FixedWidthTypes.Boolean, spec.Shape), nil
	case "string", "language":
		return arrow.BinaryTypes.String, nil
	default:
		return nil, fmt.Errorf("unsupported dtype %q", spec.DType)
	}
}

func vectorType(elem arrow.DataType, shape []int) arrow.DataType {
	if len(shape) == 0 || (len(shape) == 1 && shape[0] == 1) {
		return elem
	}
	dt := elem
	for i := len(shape) - 1; i >= 0; i-- {
		dt = arrow.FixedSizeListOf(int32(shape[i]), dt)
	}
	return dt
}

func sortedKeys(features map[string]meta.FeatureSpec) []string {
	keys := make([]string, 0, len(features))
	for k := range features {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}
