package video

import "fmt"

// VideoFrameRGB24 is the typed contract for streaming raw RGB24 into encoders.
type VideoFrameRGB24 struct {
	Data   []byte
	Width  int
	Height int
}

func (f VideoFrameRGB24) ExpectedSize() int {
	if f.Width <= 0 || f.Height <= 0 {
		return 0
	}
	return f.Width * f.Height * 3
}

func (f VideoFrameRGB24) Validate() error {
	want := f.ExpectedSize()
	if want == 0 {
		return fmt.Errorf("invalid video frame dimensions %dx%d", f.Width, f.Height)
	}
	if len(f.Data) != want {
		return fmt.Errorf("rgb24 frame size mismatch: got %d want %d", len(f.Data), want)
	}
	return nil
}

// ParseOptionalVideoFrameRGB24 parses a frame value; nil/absent input returns ok=false without error.
func ParseOptionalVideoFrameRGB24(v any, width, height int) (VideoFrameRGB24, bool, error) {
	if v == nil {
		return VideoFrameRGB24{}, false, nil
	}
	frame, err := AsVideoFrameRGB24(v, width, height)
	if err != nil {
		return VideoFrameRGB24{}, false, err
	}
	return frame, true, nil
}

// AsVideoFrameRGB24 accepts typed frames or legacy []byte with explicit dimensions.
func AsVideoFrameRGB24(v any, width, height int) (VideoFrameRGB24, error) {
	switch x := v.(type) {
	case VideoFrameRGB24:
		if err := x.Validate(); err != nil {
			return VideoFrameRGB24{}, err
		}
		return x, nil
	case []byte:
		f := VideoFrameRGB24{Data: x, Width: width, Height: height}
		if err := f.Validate(); err != nil {
			return VideoFrameRGB24{}, err
		}
		return f, nil
	default:
		return VideoFrameRGB24{}, fmt.Errorf("unsupported video frame type %T", v)
	}
}
