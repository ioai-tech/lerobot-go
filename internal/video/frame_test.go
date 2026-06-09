package video

import "testing"

func TestVideoFrameRGB24Validate(t *testing.T) {
	f := VideoFrameRGB24{Data: make([]byte, 12), Width: 2, Height: 2}
	if err := f.Validate(); err != nil {
		t.Fatalf("valid frame rejected: %v", err)
	}
	bad := VideoFrameRGB24{Data: make([]byte, 10), Width: 2, Height: 2}
	if err := bad.Validate(); err == nil {
		t.Fatal("expected size mismatch error")
	}
}

func TestAsVideoFrameRGB24LegacyBytes(t *testing.T) {
	raw := make([]byte, 12)
	got, err := AsVideoFrameRGB24(raw, 2, 2)
	if err != nil {
		t.Fatalf("AsVideoFrameRGB24 failed: %v", err)
	}
	if len(got.Data) != 12 {
		t.Fatalf("unexpected data len %d", len(got.Data))
	}
}
