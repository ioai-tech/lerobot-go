package stats

import (
	"encoding/json"
	"math"
	"testing"
)

func TestAggregateStats(t *testing.T) {
	a := EpisodeStats{
		"observation.state": {
			"min": []float64{0}, "max": []float64{1}, "mean": []float64{0.5},
			"std": []float64{0.1}, "count": []int64{10},
		},
	}
	b := EpisodeStats{
		"observation.state": {
			"min": []float64{-1}, "max": []float64{2}, "mean": []float64{1.0},
			"std": []float64{0.2}, "count": []int64{10},
		},
	}
	agg := AggregateStats([]EpisodeStats{a, b})
	if agg == nil {
		t.Fatal("nil aggregate")
	}
	st := agg["observation.state"]
	if st["count"].([]int64)[0] != 20 {
		t.Fatalf("count=%v", st["count"])
	}
}

func TestAggregateStatsImageNoDoubleDivide(t *testing.T) {
	mean := ImageStat311{{{0.50}}, {{0.10}}, {{0.20}}}
	a := EpisodeStats{
		"cam": {
			"min": mean, "max": mean, "mean": mean,
			"std":   ImageStat311{{{0.01}}, {{0.01}}, {{0.01}}},
			"count": []int64{10},
		},
	}
	agg := AggregateStats([]EpisodeStats{a, a})
	got, ok := AsImageStat311(agg["cam"]["mean"])
	if !ok {
		t.Fatalf("mean type %T", agg["cam"]["mean"])
	}
	ch := got.Channels()
	if len(ch) != 3 {
		t.Fatalf("channels=%d", len(ch))
	}
	if ch[0] < 0.49 || ch[0] > 0.51 {
		t.Fatalf("mean R=%v want ~0.50 (double /255 would be ~0.002)", ch[0])
	}
}

func TestComputeVectorStatsSanitizesInf(t *testing.T) {
	fs := computeVectorStats([]float64{1, math.Inf(1), math.NaN()}, nil)
	data, err := json.Marshal(fs)
	if err != nil {
		t.Fatalf("marshal stats: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty json")
	}
}
