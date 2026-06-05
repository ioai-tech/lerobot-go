package video

import "testing"

func TestPathLocator(t *testing.T) {
	l := NewLocator(Config{})
	_, err := l.FFmpegPath()
	if err == nil {
		t.Skip("ffmpeg on PATH")
	}
}

func TestExplicitLocator(t *testing.T) {
	l := NewLocator(Config{FFmpegPath: "/custom/ffmpeg", FFprobePath: "/custom/ffprobe"})
	p, err := l.FFmpegPath()
	if err != nil || p != "/custom/ffmpeg" {
		t.Fatalf("got %q err=%v", p, err)
	}
}
