package parquetx

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
	"github.com/ioai-tech/lerobot-go/internal/meta"
)

// AppendWriter mirrors Python pq.ParquetWriter with snappy + dictionary.
type AppendWriter struct {
	path   string
	schema *arrow.Schema
	alloc  memory.Allocator
	writer *pqarrow.FileWriter
	file   *os.File
}

func NewAppendWriterWithFeatures(path string, schema *arrow.Schema, features map[string]meta.FeatureSpec) (*AppendWriter, error) {
	if features != nil {
		var err error
		schema, err = SchemaWithHFMetadata(schema, features)
		if err != nil {
			return nil, err
		}
	}
	return NewAppendWriter(path, schema)
}

func NewAppendWriter(path string, schema *arrow.Schema) (*AppendWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	props := parquet.NewWriterProperties(
		parquet.WithCompression(compress.Codecs.Snappy),
		parquet.WithDictionaryDefault(true),
	)
	aw := &AppendWriter{
		path:   path,
		schema: schema,
		alloc:  memory.NewGoAllocator(),
		file:   f,
	}
	w, err := pqarrow.NewFileWriter(schema, f, props, pqarrow.DefaultWriterProps())
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	aw.writer = w
	return aw, nil
}

func OpenAppendWriter(path string, schema *arrow.Schema) (*AppendWriter, error) {
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("append to existing parquet not supported in-place; use merger rotation")
	}
	return NewAppendWriter(path, schema)
}

func (w *AppendWriter) WriteEpisodeColumns(columns map[string]any, length int, features map[string]meta.FeatureSpec) error {
	rec, err := buildRecord(w.alloc, w.schema, columns, length, features)
	if err != nil {
		return err
	}
	defer rec.Release()
	return w.writer.Write(rec)
}

func (w *AppendWriter) Close() error {
	if w.writer != nil {
		err := w.writer.Close()
		w.writer = nil
		w.file = nil // pqarrow.FileWriter.Close closes the underlying file
		return err
	}
	return nil
}

func buildRecord(alloc memory.Allocator, schema *arrow.Schema, columns map[string]any, length int, features map[string]meta.FeatureSpec) (arrow.RecordBatch, error) {
	arrays := make([]arrow.Array, len(schema.Fields()))
	defer func() {
		for _, arr := range arrays {
			if arr != nil {
				arr.Release()
			}
		}
	}()

	for i, field := range schema.Fields() {
		spec := features[field.Name]
		vals, ok := columns[field.Name]
		if !ok {
			return nil, fmt.Errorf("missing column %q", field.Name)
		}
		if spec.DType == "image" {
			cells, ok := vals.([]ImageCell)
			if !ok {
				return nil, fmt.Errorf("column %q: expected []ImageCell", field.Name)
			}
			arr, err := buildImageArray(alloc, cells)
			if err != nil {
				return nil, fmt.Errorf("column %q: %w", field.Name, err)
			}
			arrays[i] = arr
			continue
		}
		b := array.NewBuilder(alloc, field.Type)
		if err := appendColumn(b, vals, length, spec); err != nil {
			b.Release()
			return nil, fmt.Errorf("column %q: %w", field.Name, err)
		}
		arrays[i] = b.NewArray()
		b.Release()
	}

	outArrays := make([]arrow.Array, len(arrays))
	copy(outArrays, arrays)
	for i := range arrays {
		arrays[i] = nil
	}
	rec := array.NewRecordBatch(schema, outArrays, int64(length))
	rec.Retain()
	return rec, nil
}

func appendColumn(b array.Builder, vals any, length int, spec meta.FeatureSpec) error {
	switch spec.DType {
	case "string", "language":
		sb := b.(*array.StringBuilder)
		ss, ok := vals.([]string)
		if !ok {
			return fmt.Errorf("expected []string")
		}
		for _, s := range ss {
			sb.Append(s)
		}
	case "float32":
		return appendNumeric(b, vals, length, spec, func(bb array.Builder, v float64) {
			bb.(*array.Float32Builder).Append(float32(v))
		})
	case "float64":
		return appendNumeric(b, vals, length, spec, func(bb array.Builder, v float64) {
			bb.(*array.Float64Builder).Append(v)
		})
	case "int64":
		return appendNumeric(b, vals, length, spec, func(bb array.Builder, v float64) {
			bb.(*array.Int64Builder).Append(int64(v))
		})
	case "int32":
		return appendNumeric(b, vals, length, spec, func(bb array.Builder, v float64) {
			bb.(*array.Int32Builder).Append(int32(v))
		})
	default:
		return fmt.Errorf("unsupported dtype %q", spec.DType)
	}
	return nil
}

type numAppender func(array.Builder, float64)

func appendNumeric(b array.Builder, vals any, length int, spec meta.FeatureSpec, appendScalar numAppender) error {
	if len(spec.Shape) == 0 || (len(spec.Shape) == 1 && spec.Shape[0] == 1) {
		switch data := vals.(type) {
		case []float32:
			for _, v := range data {
				appendScalar(b, float64(v))
			}
		case []float64:
			for _, v := range data {
				appendScalar(b, v)
			}
		case []int64:
			for _, v := range data {
				appendScalar(b, float64(v))
			}
		case []int:
			for _, v := range data {
				appendScalar(b, float64(v))
			}
		default:
			return fmt.Errorf("unexpected type %T", vals)
		}
		return nil
	}
	// fixed-size list features
	switch data := vals.(type) {
	case [][]float32:
		lb := b.(*array.FixedSizeListBuilder)
		inner := lb.ValueBuilder()
		for _, row := range data {
			lb.Append(true)
			for _, v := range row {
				appendScalar(inner, float64(v))
			}
		}
	case [][]float64:
		lb := b.(*array.FixedSizeListBuilder)
		inner := lb.ValueBuilder()
		for _, row := range data {
			lb.Append(true)
			for _, v := range row {
				appendScalar(inner, v)
			}
		}
	case [][]int64:
		lb := b.(*array.FixedSizeListBuilder)
		inner := lb.ValueBuilder()
		for _, row := range data {
			lb.Append(true)
			for _, v := range row {
				appendScalar(inner, float64(v))
			}
		}
	default:
		return fmt.Errorf("unexpected vector type %T", vals)
	}
	_ = length
	return nil
}
