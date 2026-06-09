package video

import (
	"encoding/json"
	"testing"
)

func TestFFprobeIntUnmarshalString(t *testing.T) {
	var s ffprobeStream
	if err := json.Unmarshal([]byte(`{"bits_per_raw_sample":"16"}`), &s); err != nil {
		t.Fatal(err)
	}
	if int(s.BitsPerSample) != 16 {
		t.Fatalf("got %d want 16", s.BitsPerSample)
	}
}

func TestFFprobeIntUnmarshalNumber(t *testing.T) {
	var s ffprobeStream
	if err := json.Unmarshal([]byte(`{"bits_per_raw_sample":24}`), &s); err != nil {
		t.Fatal(err)
	}
	if int(s.BitsPerSample) != 24 {
		t.Fatalf("got %d want 24", s.BitsPerSample)
	}
}
