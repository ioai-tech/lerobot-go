package lerobot

import "github.com/ioai-tech/lerobot-go/internal/video"

// VideoFrameRGB24 is the typed RGB24 payload for video dtype features.
type VideoFrameRGB24 = video.VideoFrameRGB24

// NewVideoFrameRGB24 validates and constructs a video frame payload.
func NewVideoFrameRGB24(data []byte, width, height int) (VideoFrameRGB24, error) {
	frame := VideoFrameRGB24{Data: data, Width: width, Height: height}
	return frame, frame.Validate()
}
