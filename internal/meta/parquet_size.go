package meta

import (
	"os"

	"github.com/apache/arrow-go/v18/parquet/file"
)

// FileSizeMB returns on-disk file size in megabytes.
func FileSizeMB(path string) (float64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return float64(info.Size()) / (1024 * 1024), nil
}

// ParquetUncompressedSizeMB mirrors io_utils.get_parquet_file_size_in_mb (uncompressed bytes).
func ParquetUncompressedSizeMB(path string) (float64, error) {
	reader, err := file.OpenParquetFile(path, false)
	if err != nil {
		return 0, err
	}
	defer func() { _ = reader.Close() }()

	meta := reader.MetaData()
	var total int64
	for rg := 0; rg < meta.NumRowGroups(); rg++ {
		rgMeta := meta.RowGroup(rg)
		for col := 0; col < rgMeta.NumColumns(); col++ {
			chunk, err := rgMeta.ColumnChunk(col)
			if err != nil {
				return 0, err
			}
			total += chunk.TotalUncompressedSize()
		}
	}
	return float64(total) / (1024 * 1024), nil
}
