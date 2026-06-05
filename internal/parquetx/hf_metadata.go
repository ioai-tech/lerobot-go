package parquetx

import (
	"encoding/json"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/ioai-tech/lerobot-go/internal/meta"
)

func BuildHFSchemaMetadata(features map[string]meta.FeatureSpec) (arrow.Metadata, error) {
	featMeta := map[string]any{}
	names := sortedKeys(features)
	for _, name := range names {
		spec := features[name]
		featMeta[name] = hfFeatureEntry(spec)
	}
	payload := map[string]any{
		"info": map[string]any{
			"features": featMeta,
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return arrow.Metadata{}, err
	}
	return arrow.NewMetadata([]string{"huggingface"}, []string{string(data)}), nil
}

func hfFeatureEntry(spec meta.FeatureSpec) any {
	switch spec.DType {
	case "image":
		return map[string]any{"_type": "Image"}
	case "string", "language":
		return map[string]any{"dtype": "string", "_type": "Value"}
	default:
		if len(spec.Shape) == 0 || (len(spec.Shape) == 1 && spec.Shape[0] == 1) {
			return map[string]any{"dtype": spec.DType, "_type": "Value"}
		}
		if len(spec.Shape) == 1 {
			return map[string]any{
				"feature": map[string]any{"dtype": spec.DType, "_type": "Value"},
				"length":  spec.Shape[0],
				"_type":   "Sequence",
			}
		}
		return map[string]any{
			"shape": spec.Shape,
			"dtype": spec.DType,
			"_type": "Array" + shapeRankName(len(spec.Shape)),
		}
	}
}

func shapeRankName(rank int) string {
	switch rank {
	case 2:
		return "2D"
	case 3:
		return "3D"
	case 4:
		return "4D"
	case 5:
		return "5D"
	default:
		return "2D"
	}
}

func SchemaWithHFMetadata(schema *arrow.Schema, features map[string]meta.FeatureSpec) (*arrow.Schema, error) {
	md, err := BuildHFSchemaMetadata(features)
	if err != nil {
		return nil, err
	}
	return arrow.NewSchema(schema.Fields(), &md), nil
}
