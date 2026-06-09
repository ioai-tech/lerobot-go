package buffer

import (
	"math"
	"testing"

	"github.com/ioai-tech/lerobot-go/internal/meta"
)

func TestAppendFloat64RowDeepCopy(t *testing.T) {
	features := map[string]meta.FeatureSpec{
		"observation.state": {DType: "float64", Shape: []int{3}},
	}
	b := New(0, 30, features)
	shared := []float64{1, 2, 3}
	for i := 0; i < 3; i++ {
		if err := b.AddFrame(map[string]any{
			"task":              "pick",
			"observation.state": shared,
		}); err != nil {
			t.Fatal(err)
		}
		for j := range shared {
			shared[j] = float64(i*10 + j + 100)
		}
	}
	rows := b.columns["observation.state"].([][]float64)
	if len(rows) != 3 {
		t.Fatalf("rows=%d want 3", len(rows))
	}
	want := [][]float64{
		{1, 2, 3},
		{100, 101, 102},
		{110, 111, 112},
	}
	for i, row := range rows {
		for j, v := range row {
			if v != want[i][j] {
				t.Fatalf("row[%d][%d]=%v want %v (slice alias corrupted stored rows)", i, j, v, want[i][j])
			}
		}
	}
}

func TestObservationStateValuesStayFiniteThroughParquetPath(t *testing.T) {
	features := map[string]meta.FeatureSpec{
		"observation.state": {DType: "float64", Shape: []int{4}},
		"action":            {DType: "float64", Shape: []int{4}},
	}
	b := New(0, 30, features)
	reuse := make([]float64, 4)
	for i := 0; i < 20; i++ {
		for j := range reuse {
			reuse[j] = float64(i)*0.1 + float64(j)*0.01
		}
		if err := b.AddFrame(map[string]any{
			"task":              "move",
			"observation.state": reuse,
			"action":            reuse,
		}); err != nil {
			t.Fatal(err)
		}
	}
	stateRows := b.columns["observation.state"].([][]float64)
	for i, row := range stateRows {
		for j, v := range row {
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 100 {
				t.Fatalf("state[%d][%d]=%v out of sane joint range", i, j, v)
			}
			want := float64(i)*0.1 + float64(j)*0.01
			if v != want {
				t.Fatalf("state[%d][%d]=%v want %v", i, j, v, want)
			}
		}
	}
}
