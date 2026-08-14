package media

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"volnorez/internal/tools"
)

type InputInfo struct {
	Duration    time.Duration
	HasAudio    bool
	Title       string
	CoverStream int
}

type Selection struct {
	Start    time.Duration
	Duration time.Duration
}

type probeOutput struct {
	Format struct {
		Duration string `json:"duration"`
		Tags     struct {
			Title string `json:"title"`
		} `json:"tags"`
	} `json:"format"`
	Streams []struct {
		Index       int    `json:"index"`
		CodecType   string `json:"codec_type"`
		Disposition struct {
			AttachedPicture int `json:"attached_pic"`
		} `json:"disposition"`
	} `json:"streams"`
}

func Probe(ctx context.Context, r tools.Runner, ffprobe, input string) (InputInfo, error) {
	result, err := r.Run(ctx, tools.Command{
		Name: ffprobe,
		Args: []string{
			"-v", "error",
			"-show_entries", "format=duration:format_tags=title:stream=index,codec_type:stream_disposition=attached_pic",
			"-of", "json",
			input,
		},
	})
	if err != nil {
		return InputInfo{}, fmt.Errorf("probe input: %w", err)
	}

	var output probeOutput
	if err := json.Unmarshal(result.Stdout, &output); err != nil {
		return InputInfo{}, fmt.Errorf("parse ffprobe output: %w", err)
	}
	durationSeconds, err := strconv.ParseFloat(output.Format.Duration, 64)
	if err != nil {
		return InputInfo{}, fmt.Errorf("parse input duration %q: %w", output.Format.Duration, err)
	}
	if math.IsNaN(durationSeconds) || math.IsInf(durationSeconds, 0) {
		return InputInfo{}, fmt.Errorf("parse input duration %q", output.Format.Duration)
	}
	duration := time.Duration(durationSeconds * float64(time.Second))
	if duration <= 0 {
		return InputInfo{}, fmt.Errorf("input duration %q must be positive", output.Format.Duration)
	}

	info := InputInfo{Duration: duration, Title: output.Format.Tags.Title, CoverStream: -1}
	for _, stream := range output.Streams {
		if stream.CodecType == "audio" {
			info.HasAudio = true
		}
		if stream.Disposition.AttachedPicture == 1 {
			info.CoverStream = stream.Index
		}
	}
	if !info.HasAudio {
		return InputInfo{}, fmt.Errorf("input has no audio stream")
	}
	return info, nil
}

func SelectInterval(info InputInfo, start, duration time.Duration, hasDuration bool) (Selection, error) {
	if start < 0 || start >= info.Duration {
		return Selection{}, fmt.Errorf("invalid start %s for input duration %s", start, info.Duration)
	}
	if !hasDuration {
		return Selection{Start: start, Duration: info.Duration - start}, nil
	}
	if duration <= 0 {
		return Selection{}, fmt.Errorf("invalid duration %s for input duration %s", duration, info.Duration)
	}
	if duration > info.Duration-start {
		return Selection{}, fmt.Errorf("interval start %s with duration %s exceeds input duration %s", start, duration, info.Duration)
	}
	return Selection{Start: start, Duration: duration}, nil
}
