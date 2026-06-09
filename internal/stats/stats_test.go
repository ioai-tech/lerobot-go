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
