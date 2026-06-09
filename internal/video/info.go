package video

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
)

type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
}

type ffprobeStream struct {
	CodecType     string     `json:"codec_type"`
	CodecName     string     `json:"codec_name"`
	Width         int        `json:"width"`
	Height        int        `json:"height"`
	PixFmt        string     `json:"pix_fmt"`
	RFrameRate    string     `json:"r_frame_rate"`
	Channels      int        `json:"channels"`
	BitRate       string     `json:"bit_rate"`
	SampleRate    string     `json:"sample_rate"`
	BitsPerSample ffprobeInt `json:"bits_per_raw_sample"`
	ChannelLayout string     `json:"channel_layout"`
}

// ffprobeInt accepts ffprobe numeric fields encoded as JSON numbers or strings.
type ffprobeInt int

func (f *ffprobeInt) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*f = 0
		return nil
	}
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		*f = ffprobeInt(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s == "" {
			*f = 0
			return nil
		}
		v, err := strconv.Atoi(s)
		if err != nil {
			return err
		}
		*f = ffprobeInt(v)
		return nil
	}
	return nil
}

// GetVideoInfo mirrors lerobot.datasets.video_utils.get_video_info (v0.5.1).
func GetVideoInfo(ctx context.Context, locator Locator, videoPath string) (map[string]any, error) {
	ffprobe, err := locator.FFprobePath()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, ffprobe,
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		videoPath,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe %s: %w", videoPath, err)
	}
	var parsed ffprobeOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, err
	}

	info := map[string]any{}
	var videoStream *ffprobeStream
	var audioStream *ffprobeStream
	for i := range parsed.Streams {
		s := &parsed.Streams[i]
		switch s.CodecType {
		case "video":
			if videoStream == nil {
				videoStream = s
			}
		case "audio":
			if audioStream == nil {
				audioStream = s
			}
		}
	}
	if videoStream == nil {
		return info, nil
	}

	channels, err := pixelChannels(videoStream.PixFmt)
	if err != nil {
		return nil, err
	}
	fps, _ := parseFrameRate(videoStream.RFrameRate)

	info["video.height"] = videoStream.Height
	info["video.width"] = videoStream.Width
	info["video.codec"] = videoStream.CodecName
	info["video.pix_fmt"] = videoStream.PixFmt
	info["video.is_depth_map"] = false
	info["video.fps"] = fps
	info["video.channels"] = channels

	if audioStream == nil {
		info["has_audio"] = false
		return info, nil
	}
	info["has_audio"] = true
	info["audio.channels"] = audioStream.Channels
	info["audio.codec"] = audioStream.CodecName
	if audioStream.BitRate != "" {
		if br, err := strconv.Atoi(audioStream.BitRate); err == nil {
			info["audio.bit_rate"] = br
		}
	}
	if audioStream.SampleRate != "" {
		if sr, err := strconv.Atoi(audioStream.SampleRate); err == nil {
			info["audio.sample_rate"] = sr
		}
	}
	if audioStream.BitsPerSample > 0 {
		info["audio.bit_depth"] = int(audioStream.BitsPerSample)
	}
	if audioStream.ChannelLayout != "" {
		info["audio.channel_layout"] = audioStream.ChannelLayout
	}
	return info, nil
}

func pixelChannels(pixFmt string) (int, error) {
	pixFmt = strings.ToLower(pixFmt)
	switch {
	case strings.Contains(pixFmt, "gray"), strings.Contains(pixFmt, "depth"), strings.Contains(pixFmt, "monochrome"):
		return 1, nil
	case strings.Contains(pixFmt, "rgba"), strings.Contains(pixFmt, "yuva"):
		return 4, nil
	case strings.Contains(pixFmt, "rgb"), strings.Contains(pixFmt, "yuv"):
		return 3, nil
	default:
		return 0, fmt.Errorf("unknown pix_fmt %q", pixFmt)
	}
}

func parseFrameRate(rate string) (float64, error) {
	if rate == "" || rate == "0/0" {
		return 0, nil
	}
	parts := strings.Split(rate, "/")
	if len(parts) != 2 {
		v, err := strconv.ParseFloat(rate, 64)
		return v, err
	}
	num, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, err
	}
	den, err := strconv.ParseFloat(parts[1], 64)
	if err != nil || den == 0 {
		return 0, err
	}
	return math.Round(num / den), nil
}
