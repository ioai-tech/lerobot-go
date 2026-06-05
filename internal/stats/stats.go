package stats

import (
	"math"
	"sort"
)

var DefaultQuantiles = []float64{0.01, 0.10, 0.50, 0.90, 0.99}

type FeatureStats map[string]any

type EpisodeStats map[string]FeatureStats

type FeatureDesc struct {
	DType string
	Shape []int
}

type EpisodeInput struct {
	Columns    map[string]any
	FramePaths map[string][]string
	FrameBytes map[string][][]byte
}

func ComputeEpisodeStats(in EpisodeInput, features map[string]FeatureDesc, opts Options) EpisodeStats {
	opts = opts.normalized()
	out := make(EpisodeStats)
	for key, spec := range features {
		switch spec.DType {
		case "string", "language":
			continue
		case "image", "video":
			paths := in.FramePaths[key]
			frames := in.FrameBytes[key]
			if len(paths) == 0 && len(frames) == 0 {
				continue
			}
			full := opts.Mode == ModeFull
			var (
				imgs        [][][][]uint8
				sampleCount int
				err         error
			)
			if len(paths) > 0 {
				imgs, sampleCount, err = sampleImages(paths, full)
			} else {
				imgs, sampleCount, err = sampleImageBytes(frames, full)
			}
			if err != nil || sampleCount == 0 {
				continue
			}
			out[key] = computeImageFeatureStats(imgs, sampleCount)
		default:
			vals, ok := in.Columns[key]
			if !ok {
				continue
			}
			out[key] = computeVectorStats(vals, spec.Shape)
		}
	}
	return out
}

func computeImageFeatureStats(imgs [][][][]uint8, sampleCount int) FeatureStats {
	// imgs: N x C x H x W
	if len(imgs) == 0 {
		return FeatureStats{"count": []int64{0}}
	}
	channels := len(imgs[0])
	flat := make([][]float64, 0)
	for _, nchw := range imgs {
		for y := range nchw[0] {
			for x := range nchw[0][y] {
				row := make([]float64, channels)
				for c := 0; c < channels; c++ {
					row[c] = float64(nchw[c][y][x])
				}
				flat = append(flat, row)
			}
		}
	}
	raw := computeFlatStats(flat, sampleCount)
	result := make(FeatureStats, len(raw))
	for k, v := range raw {
		if k == "count" {
			result[k] = v
			continue
		}
		ch := asFloat64Slice(v)
		result[k] = ImageStat311FromChannels(ch)
	}
	return result
}

func computeVectorStats(vals any, shape []int) FeatureStats {
	flat := flattenRows(vals)
	if len(flat) == 0 {
		return FeatureStats{"count": []int64{0}}
	}
	return computeFlatStats(flat, len(flat))
}

func computeFlatStats(flat [][]float64, sampleCount int) FeatureStats {
	dim := len(flat[0])
	count := float64(len(flat))
	min := make([]float64, dim)
	max := make([]float64, dim)
	sum := make([]float64, dim)
	sumSq := make([]float64, dim)
	for i := range min {
		min[i] = flat[0][i]
		max[i] = flat[0][i]
	}
	for _, row := range flat {
		for i, v := range row {
			if v < min[i] {
				min[i] = v
			}
			if v > max[i] {
				max[i] = v
			}
			sum[i] += v
			sumSq[i] += v * v
		}
	}
	mean := make([]float64, dim)
	std := make([]float64, dim)
	for i := range mean {
		mean[i] = sum[i] / count
		variance := sumSq[i]/count - mean[i]*mean[i]
		if variance < 0 {
			variance = 0
		}
		std[i] = math.Sqrt(variance)
	}
	result := FeatureStats{
		"min":   append([]float64(nil), min...),
		"max":   append([]float64(nil), max...),
		"mean":  append([]float64(nil), mean...),
		"std":   append([]float64(nil), std...),
		"count": []int64{int64(sampleCount)},
	}
	if len(flat) < 2 {
		for _, q := range DefaultQuantiles {
			result[quantileKey(q)] = append([]float64(nil), mean...)
		}
		return result
	}
	for _, q := range DefaultQuantiles {
		result[quantileKey(q)] = quantilePerDim(flat, q)
	}
	return result
}

func quantilePerDim(flat [][]float64, q float64) []float64 {
	dim := len(flat[0])
	out := make([]float64, dim)
	for d := 0; d < dim; d++ {
		vals := make([]float64, len(flat))
		for i, row := range flat {
			vals[i] = row[d]
		}
		sort.Float64s(vals)
		out[d] = percentile(vals, q)
	}
	return out
}

func percentile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	pos := q * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	frac := pos - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// ImageStat311 is the (3,1,1) nested layout used by LeRobot image/video stats.
type ImageStat311 [][][]float64

func ImageStat311FromChannels(ch []float64) ImageStat311 {
	out := make(ImageStat311, len(ch))
	for i, v := range ch {
		out[i] = [][]float64{{v / 255.0}}
	}
	return out
}

func (s ImageStat311) Channels() []float64 {
	out := make([]float64, len(s))
	for i, plane := range s {
		if len(plane) > 0 && len(plane[0]) > 0 {
			out[i] = plane[0][0]
		}
	}
	return out
}

func quantileKey(q float64) string {
	return "q" + formatPct(int(q*100))
}

func formatPct(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func flattenRows(vals any) [][]float64 {
	switch data := vals.(type) {
	case []float32:
		out := make([][]float64, len(data))
		for i, v := range data {
			out[i] = []float64{float64(v)}
		}
		return out
	case []float64:
		out := make([][]float64, len(data))
		for i, v := range data {
			out[i] = []float64{v}
		}
		return out
	case []int64:
		out := make([][]float64, len(data))
		for i, v := range data {
			out[i] = []float64{float64(v)}
		}
		return out
	case [][]float32:
		out := make([][]float64, len(data))
		for i, row := range data {
			out[i] = make([]float64, len(row))
			for j, v := range row {
				out[i][j] = float64(v)
			}
		}
		return out
	case [][]float64:
		return data
	case [][]int64:
		out := make([][]float64, len(data))
		for i, row := range data {
			out[i] = make([]float64, len(row))
			for j, v := range row {
				out[i][j] = float64(v)
			}
		}
		return out
	default:
		return nil
	}
}

func AggregateStats(list []EpisodeStats) map[string]map[string]any {
	if len(list) == 0 {
		return nil
	}
	keys := map[string]struct{}{}
	for _, ep := range list {
		for k := range ep {
			keys[k] = struct{}{}
		}
	}
	out := make(map[string]map[string]any, len(keys))
	for key := range keys {
		var parts []FeatureStats
		for _, ep := range list {
			if s, ok := ep[key]; ok {
				parts = append(parts, s)
			}
		}
		out[key] = aggregateFeature(parts)
	}
	return out
}

func asFloat64Slice(v any) []float64 {
	switch x := v.(type) {
	case []float64:
		return x
	case ImageStat311:
		return x.Channels()
	case []int64:
		out := make([]float64, len(x))
		for i, n := range x {
			out[i] = float64(n)
		}
		return out
	case []int:
		out := make([]float64, len(x))
		for i, n := range x {
			out[i] = float64(n)
		}
		return out
	case []any:
		out := make([]float64, len(x))
		for i, e := range x {
			out[i] = toFloat64(e)
		}
		return out
	default:
		return nil
	}
}

func toFloat64(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	default:
		return 0
	}
}

func aggregateFeature(parts []FeatureStats) map[string]any {
	if len(parts) == 0 {
		return nil
	}
	dim := len(asFloat64Slice(parts[0]["mean"]))
	totalCount := 0.0
	weightedMean := make([]float64, dim)
	weightedVar := make([]float64, dim)
	min := make([]float64, dim)
	max := make([]float64, dim)
	for i, p := range parts {
		c := countValue(p["count"])
		m := asFloat64Slice(p["mean"])
		s := asFloat64Slice(p["std"])
		if i == 0 {
			copy(min, asFloat64Slice(p["min"]))
			copy(max, asFloat64Slice(p["max"]))
		} else {
			pmin := asFloat64Slice(p["min"])
			pmax := asFloat64Slice(p["max"])
			for j := range min {
				if pmin[j] < min[j] {
					min[j] = pmin[j]
				}
				if pmax[j] > max[j] {
					max[j] = pmax[j]
				}
			}
		}
		for j := range m {
			weightedMean[j] += m[j] * c
			weightedVar[j] += (s[j]*s[j] + m[j]*m[j]) * c
		}
		totalCount += c
	}
	mean := make([]float64, dim)
	std := make([]float64, dim)
	for j := range mean {
		mean[j] = weightedMean[j] / totalCount
		variance := weightedVar[j]/totalCount - mean[j]*mean[j]
		if variance < 0 {
			variance = 0
		}
		std[j] = math.Sqrt(variance)
	}
	isImage := isImageStat(parts[0]["mean"])
	result := map[string]any{
		"count": []int64{int64(totalCount)},
	}
	if isImage {
		result["min"] = ImageStat311FromChannels(min)
		result["max"] = ImageStat311FromChannels(max)
		result["mean"] = ImageStat311FromChannels(mean)
		result["std"] = ImageStat311FromChannels(std)
	} else {
		result["min"] = min
		result["max"] = max
		result["mean"] = mean
		result["std"] = std
	}
	for _, q := range DefaultQuantiles {
		key := quantileKey(q)
		qvals := make([]float64, dim)
		for _, p := range parts {
			c := countValue(p["count"])
			qv := asFloat64Slice(p[key])
			if len(qv) == 0 {
				qv = asFloat64Slice(p["mean"])
			}
			for j := range qvals {
				if j < len(qv) {
					qvals[j] += qv[j] * c
				}
			}
		}
		for j := range qvals {
			qvals[j] /= totalCount
		}
		if isImage {
			result[key] = ImageStat311FromChannels(qvals)
		} else {
			result[key] = qvals
		}
	}
	return result
}

func countValue(v any) float64 {
	switch x := v.(type) {
	case []int64:
		if len(x) > 0 {
			return float64(x[0])
		}
	case []float64:
		if len(x) > 0 {
			return x[0]
		}
	case []int:
		if len(x) > 0 {
			return float64(x[0])
		}
	case []any:
		if len(x) > 0 {
			return toFloat64(x[0])
		}
	case int64:
		return float64(x)
	case float64:
		return x
	}
	return 0
}

func isImageStat(v any) bool {
	_, ok := v.(ImageStat311)
	return ok
}

func ToJSONSerializable(stats map[string]map[string]any) map[string]map[string]any {
	return stats
}
