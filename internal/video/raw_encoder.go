package video

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
)

// RawRGBEncoder streams raw RGB24 frames directly into an ffmpeg process
// for video dtype features. This avoids writing per-frame PNG files to
// tempfs/disk for the bulk of the episode, keeping memory bounded to
// in-flight decoded frames + small stats samples.
type RawRGBEncoder struct {
	locator Locator
	vcodec  string
	crf     int
	fps     int
	width   int
	height  int
	output  string

	cmd      *exec.Cmd
	stdin    io.WriteCloser
	frameCh  chan []byte
	stopCh   chan struct{}
	writerWG sync.WaitGroup
	closed   bool
	closeMu  sync.Mutex
}

func encoderQueueDepth() int {
	v := os.Getenv("LEROBOT_ENCODER_QUEUE_FRAMES")
	if v == "" {
		return 3
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 3
	}
	return n
}

func NewRawRGBEncoder(ctx context.Context, locator Locator, vcodec string, crf, fps, width, height int, outputPath string) (*RawRGBEncoder, error) {
	if locator == nil {
		locator = NewLocator(Config{})
	}
	ffmpeg, err := locator.FFmpegPath()
	if err != nil {
		return nil, err
	}
	if crf <= 0 {
		crf = DefaultCRF
	}
	if vcodec == "" {
		vcodec = "libx264"
	}

	args := []string{
		"-y",
		"-f", "rawvideo",
		"-pix_fmt", "rgb24",
		"-s", fmt.Sprintf("%dx%d", width, height),
		"-framerate", strconv.Itoa(fps),
		"-i", "pipe:0",
		"-c:v", vcodec,
		"-crf", strconv.Itoa(crf),
		"-pix_fmt", "yuv420p",
		"-movflags", "+faststart",
	}
	if threads := ResolveEncoderThreads(); threads > 0 {
		args = append(args, "-threads", strconv.Itoa(threads))
	}
	args = append(args, outputPath)

	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, err
	}

	e := &RawRGBEncoder{
		locator: locator,
		vcodec:  vcodec,
		crf:     crf,
		fps:     fps,
		width:   width,
		height:  height,
		output:  outputPath,
		cmd:     cmd,
		stdin:   stdin,
		frameCh: make(chan []byte, encoderQueueDepth()),
		stopCh:  make(chan struct{}),
	}
	e.writerWG.Add(1)
	go e.writerLoop()
	return e, nil
}

func (e *RawRGBEncoder) writerLoop() {
	defer e.writerWG.Done()
	for {
		select {
		case frame, ok := <-e.frameCh:
			if !ok {
				return
			}
			if e.stdin != nil {
				_, _ = e.stdin.Write(frame)
			}
		case <-e.stopCh:
			return
		}
	}
}

func (e *RawRGBEncoder) WriteFrame(frame VideoFrameRGB24) error {
	if err := frame.Validate(); err != nil {
		return err
	}
	return e.enqueue(frame.Data)
}

// WriteFrameBytes keeps backward compatibility for callers still passing raw bytes.
func (e *RawRGBEncoder) WriteFrameBytes(rgb24 []byte) error {
	frame := VideoFrameRGB24{Data: rgb24, Width: e.width, Height: e.height}
	return e.WriteFrame(frame)
}

func (e *RawRGBEncoder) enqueue(rgb24 []byte) error {
	e.closeMu.Lock()
	defer e.closeMu.Unlock()
	if e.closed || e.stdin == nil {
		return fmt.Errorf("encoder closed or not started")
	}
	buf := append([]byte(nil), rgb24...)
	e.frameCh <- buf
	return nil
}

func (e *RawRGBEncoder) Close() error {
	e.closeMu.Lock()
	if e.closed {
		e.closeMu.Unlock()
		e.writerWG.Wait()
		return nil
	}
	e.closed = true
	close(e.frameCh)
	e.closeMu.Unlock()
	e.writerWG.Wait()

	var firstErr error
	if e.stdin != nil {
		if err := e.stdin.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		e.stdin = nil
	}
	if e.cmd != nil {
		if err := e.cmd.Wait(); err != nil && firstErr == nil {
			firstErr = err
		}
		e.cmd = nil
	}
	return firstErr
}

func (e *RawRGBEncoder) OutputPath() string {
	return e.output
}
