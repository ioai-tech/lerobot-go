package stats

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
)

func estimateNumSamples(datasetLen, minSamples, maxSamples int, power float64) int {
	if datasetLen < minSamples {
		minSamples = datasetLen
	}
	if datasetLen == 0 {
		return 0
	}
	n := int(math.Pow(float64(datasetLen), power))
	if n < minSamples {
		n = minSamples
	}
	if n > maxSamples {
		n = maxSamples
	}
	return n
}

func sampleIndices(dataLen int, full bool) []int {
	if dataLen == 0 {
		return nil
	}
	if full {
		out := make([]int, dataLen)
		for i := range out {
			out[i] = i
		}
		return out
	}
	numSamples := estimateNumSamples(dataLen, 100, 10_000, 0.75)
	if numSamples <= 1 {
		return []int{0}
	}
	out := make([]int, numSamples)
	for i := range out {
		out[i] = int(math.Round(float64(i) * float64(dataLen-1) / float64(numSamples-1)))
	}
	return out
}

func autoDownsampleHeightWidth(chw [][][]uint8, targetSize, maxSizeThreshold int) [][][]uint8 {
	if len(chw) == 0 || len(chw[0]) == 0 || len(chw[0][0]) == 0 {
		return chw
	}
	height := len(chw[0])
	width := len(chw[0][0])
	if max(width, height) < maxSizeThreshold {
		return chw
	}
	var factor int
	if width > height {
		factor = width / targetSize
	} else {
		factor = height / targetSize
	}
	if factor < 1 {
		factor = 1
	}
	out := make([][][]uint8, len(chw))
	for c := range chw {
		h2 := (height + factor - 1) / factor
		w2 := (width + factor - 1) / factor
		plane := make([][]uint8, h2)
		for y := 0; y < h2; y++ {
			row := make([]uint8, w2)
			for x := 0; x < w2; x++ {
				row[x] = chw[c][y*factor][x*factor]
			}
			plane[y] = row
		}
		out[c] = plane
	}
	return out
}

func loadImageCHW(path string) ([][][]uint8, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return decodeImageCHW(f)
}

func loadImageCHWBytes(data []byte) ([][][]uint8, error) {
	return decodeImageCHW(bytes.NewReader(data))
}

func decodeImageCHW(r interface {
	Read([]byte) (int, error)
}) ([][][]uint8, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	width := b.Dx()
	height := b.Dy()
	channels := make([][][]uint8, 3)
	for c := 0; c < 3; c++ {
		plane := make([][]uint8, height)
		for y := 0; y < height; y++ {
			row := make([]uint8, width)
			for x := 0; x < width; x++ {
				r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
				switch c {
				case 0:
					row[x] = uint8(r >> 8)
				case 1:
					row[x] = uint8(g >> 8)
				default:
					row[x] = uint8(bl >> 8)
				}
			}
			plane[y] = row
		}
		channels[c] = plane
	}
	return channels, nil
}

// sampleImages loads channel-first uint8 images (N,C,H,W) from paths.
func sampleImages(paths []string, full bool) ([][][][]uint8, int, error) {
	indices := sampleIndices(len(paths), full)
	if len(indices) == 0 {
		return nil, 0, nil
	}
	out := make([][][][]uint8, len(indices))
	for i, idx := range indices {
		chw, err := loadImageCHW(paths[idx])
		if err != nil {
			return nil, 0, fmt.Errorf("load %s: %w", paths[idx], err)
		}
		out[i] = autoDownsampleHeightWidth(chw, 150, 300)
	}
	return out, len(indices), nil
}

func sampleImageBytes(frames [][]byte, full bool) ([][][][]uint8, int, error) {
	indices := sampleIndices(len(frames), full)
	if len(indices) == 0 {
		return nil, 0, nil
	}
	out := make([][][][]uint8, len(indices))
	for i, idx := range indices {
		chw, err := loadImageCHWBytes(frames[idx])
		if err != nil {
			return nil, 0, fmt.Errorf("decode frame %d: %w", idx, err)
		}
		out[i] = autoDownsampleHeightWidth(chw, 150, 300)
	}
	return out, len(indices), nil
}
