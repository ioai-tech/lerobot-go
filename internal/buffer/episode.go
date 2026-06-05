package buffer

import (
	"fmt"

	"github.com/ioai-tech/lerobot-go/internal/features"
	"github.com/ioai-tech/lerobot-go/internal/meta"
)

type EpisodeBuffer struct {
	EpisodeIndex int
	Features     map[string]meta.FeatureSpec
	FPS          int
	columns      map[string]any
	tasks        []string
	size         int
}

func New(episodeIndex int, fps int, features map[string]meta.FeatureSpec) *EpisodeBuffer {
	return &EpisodeBuffer{
		EpisodeIndex: episodeIndex,
		Features:     features,
		FPS:          fps,
		columns:      make(map[string]any),
	}
}

func (b *EpisodeBuffer) AddFrame(frame map[string]any) error {
	if err := features.ValidateFrameKeys(frame, b.Features); err != nil {
		return err
	}
	task, _ := frame["task"].(string)
	if task == "" {
		task, _ = frame["__task__"].(string)
	}
	b.tasks = append(b.tasks, task)

	for key, spec := range b.Features {
		if spec.DType == "video" || spec.DType == "image" {
			continue
		}
		val, ok := frame[key]
		if !ok {
			continue
		}
		if err := appendValue(b, key, val, spec); err != nil {
			return err
		}
	}
	b.size++
	ts := float32(b.size-1) / float32(b.FPS)
	appendFloat32(b, "timestamp", ts)
	appendInt64(b, "frame_index", int64(b.size-1))
	appendInt64(b, "episode_index", int64(b.EpisodeIndex))
	return nil
}

func (b *EpisodeBuffer) Size() int       { return b.size }
func (b *EpisodeBuffer) Tasks() []string { return b.tasks }

func (b *EpisodeBuffer) Columns(globalIndex int64, taskIndices []int64) map[string]any {
	out := make(map[string]any, len(b.columns)+2)
	for k, v := range b.columns {
		out[k] = v
	}
	idx := make([]int64, b.size)
	for i := range idx {
		idx[i] = globalIndex + int64(i)
	}
	out["index"] = idx
	out["task_index"] = taskIndices
	return out
}

func isScalarShape(shape []int) bool {
	return len(shape) == 0 || (len(shape) == 1 && shape[0] == 1)
}

func appendValue(b *EpisodeBuffer, key string, val any, spec meta.FeatureSpec) error {
	switch spec.DType {
	case "float32":
		if isScalarShape(spec.Shape) {
			switch v := val.(type) {
			case float32:
				appendFloat32(b, key, v)
			case float64:
				appendFloat32(b, key, float32(v))
			case int:
				appendFloat32(b, key, float32(v))
			default:
				return fmt.Errorf("%q: expected scalar float32", key)
			}
			return nil
		}
		row, ok := toFloat32Row(val)
		if !ok {
			return fmt.Errorf("%q: expected []float32", key)
		}
		appendFloat32Row(b, key, row)
	case "float64":
		if isScalarShape(spec.Shape) {
			if v, ok := val.(float64); ok {
				appendFloat64(b, key, v)
				return nil
			}
			return fmt.Errorf("%q: expected scalar float64", key)
		}
		row, ok := val.([]float64)
		if !ok {
			return fmt.Errorf("%q: expected []float64", key)
		}
		appendFloat64Row(b, key, row)
	case "int64", "int32":
		if isScalarShape(spec.Shape) {
			switch v := val.(type) {
			case int64:
				appendInt64(b, key, v)
			case int:
				appendInt64(b, key, int64(v))
			default:
				return fmt.Errorf("%q: expected scalar int", key)
			}
			return nil
		}
		row, ok := val.([]int64)
		if !ok {
			return fmt.Errorf("%q: expected []int64", key)
		}
		appendInt64Row(b, key, row)
	case "string", "language":
		s, ok := val.(string)
		if !ok {
			return fmt.Errorf("%q: expected string", key)
		}
		appendString(b, key, s)
	default:
		return fmt.Errorf("unsupported dtype %q for %q", spec.DType, key)
	}
	return nil
}

func appendFloat32Row(b *EpisodeBuffer, key string, row []float32) {
	if _, ok := b.columns[key]; !ok {
		b.columns[key] = [][]float32{}
	}
	b.columns[key] = append(b.columns[key].([][]float32), row)
}

func appendFloat64Row(b *EpisodeBuffer, key string, row []float64) {
	if _, ok := b.columns[key]; !ok {
		b.columns[key] = [][]float64{}
	}
	b.columns[key] = append(b.columns[key].([][]float64), row)
}

func appendInt64Row(b *EpisodeBuffer, key string, row []int64) {
	if _, ok := b.columns[key]; !ok {
		b.columns[key] = [][]int64{}
	}
	b.columns[key] = append(b.columns[key].([][]int64), row)
}

func appendFloat64(b *EpisodeBuffer, key string, val float64) {
	if _, ok := b.columns[key]; !ok {
		b.columns[key] = []float64{}
	}
	b.columns[key] = append(b.columns[key].([]float64), val)
}

func toFloat32Row(val any) ([]float32, bool) {
	switch v := val.(type) {
	case []float32:
		return v, true
	case []float64:
		out := make([]float32, len(v))
		for i, x := range v {
			out[i] = float32(x)
		}
		return out, true
	default:
		return nil, false
	}
}

func appendFloat32(b *EpisodeBuffer, key string, val float32) {
	if _, ok := b.columns[key]; !ok {
		b.columns[key] = []float32{}
	}
	b.columns[key] = append(b.columns[key].([]float32), val)
}

func appendInt64(b *EpisodeBuffer, key string, val int64) {
	if _, ok := b.columns[key]; !ok {
		b.columns[key] = []int64{}
	}
	b.columns[key] = append(b.columns[key].([]int64), val)
}

func appendString(b *EpisodeBuffer, key, val string) {
	if _, ok := b.columns[key]; !ok {
		b.columns[key] = []string{}
	}
	b.columns[key] = append(b.columns[key].([]string), val)
}
