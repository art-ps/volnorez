package media

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"volnorez/internal/tools"
)

func ExtractAudio(ctx context.Context, r tools.Runner, ffmpeg, input, output string, s Selection) error {
	_, err := r.Run(ctx, tools.Command{
		Name: ffmpeg,
		Args: []string{
			"-v", "error", "-y",
			"-ss", ffmpegTime(s.Start),
			"-t", ffmpegTime(s.Duration),
			"-i", input,
			"-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le",
			output,
		},
	})
	if err != nil {
		return fmt.Errorf("extract audio: %w", err)
	}
	return nil
}

func PrepareCover(ctx context.Context, r tools.Runner, ffmpeg, input, explicit, output string, coverStream int) error {
	args := []string{"-v", "error", "-y"}
	if explicit != "" {
		args = append(args, "-i", explicit)
	} else {
		if coverStream < 0 {
			return fmt.Errorf("no cover image provided or embedded in input")
		}
		args = append(args, "-i", input, "-map", "0:"+strconv.Itoa(coverStream))
	}
	args = append(args, "-frames:v", "1", output)
	_, err := r.Run(ctx, tools.Command{Name: ffmpeg, Args: args})
	if err != nil {
		return fmt.Errorf("prepare cover: %w", err)
	}
	return nil
}

func ffmpegTime(value time.Duration) string {
	return strconv.FormatFloat(value.Seconds(), 'f', 3, 64)
}
