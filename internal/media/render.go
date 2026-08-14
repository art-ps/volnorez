package media

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"volnorez/internal/tools"
)

type RenderRequest struct {
	FFmpeg    string
	Input     string
	Cover     string
	ASS       string
	FontDir   string
	WorkDir   string
	Output    string
	Accent    string
	Selection Selection
}

func BuildRenderCommand(req RenderRequest) tools.Command {
	start := strconv.FormatFloat(req.Selection.Start.Seconds(), 'f', 3, 64)
	duration := strconv.FormatFloat(req.Selection.Duration.Seconds(), 'f', 3, 64)
	accent := "0x" + strings.TrimPrefix(req.Accent, "#")
	mask := `geq=r='r(X,Y)':g='g(X,Y)':b='b(X,Y)':a='if(lte(hypot(X-W/2\,Y-H/2)\,W/2)\,255\,0)'`
	filter := strings.Join([]string{
		"color=c=0x101014:s=1080x1920:r=30:d=" + duration + "[bg]",
		"[1:v]scale=360:360,format=rgba," + mask + "[cover]",
		"[0:a]atrim=start=" + start + ":duration=" + duration + ",asetpts=PTS-STARTPTS,asplit=2[aout][aw]",
		"[aw]showwaves=s=900x420:mode=line:rate=30:colors=" + accent + ",format=rgba[wave]",
		"[bg][cover]overlay=x=(W-w)/2:y=240:eof_action=repeat[tmp1]",
		"[tmp1][wave]overlay=x=(W-w)/2:y=760:shortest=1[tmp2]",
		"[tmp2]subtitles=" + filepath.Base(req.ASS) + ":fontsdir=" + filepath.Base(req.FontDir) + "[vout]",
	}, ";")
	return tools.Command{
		Name: req.FFmpeg,
		Dir:  req.WorkDir,
		Args: []string{
			"-v", "error", "-y", "-i", req.Input, "-loop", "1", "-i", req.Cover,
			"-filter_complex", filter, "-map", "[vout]", "-map", "[aout]",
			"-s", "1080x1920", "-r", "30", "-c:v", "libx264", "-pix_fmt", "yuv420p",
			"-c:a", "aac", "-ac", "2", "-b:a", "192k", "-movflags", "+faststart",
			"-shortest", req.Output,
		},
	}
}

func Render(ctx context.Context, r tools.Runner, req RenderRequest) error {
	if _, err := r.Run(ctx, BuildRenderCommand(req)); err != nil {
		return fmt.Errorf("render output: %w", err)
	}
	return nil
}

type renderProbeOutput struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		CodecType   string `json:"codec_type"`
		CodecName   string `json:"codec_name"`
		Width       int    `json:"width"`
		Height      int    `json:"height"`
		PixelFormat string `json:"pix_fmt"`
		FrameRate   string `json:"avg_frame_rate"`
	} `json:"streams"`
}

func VerifyOutput(ctx context.Context, r tools.Runner, ffprobe, output string, expected time.Duration) error {
	result, err := r.Run(ctx, tools.Command{
		Name: ffprobe,
		Args: []string{
			"-v", "error",
			"-show_entries", "format=duration:stream=codec_type,codec_name,width,height,pix_fmt,avg_frame_rate",
			"-of", "json",
			output,
		},
	})
	if err != nil {
		return fmt.Errorf("verify output: %w", err)
	}

	var probe renderProbeOutput
	if err := json.Unmarshal(result.Stdout, &probe); err != nil {
		return fmt.Errorf("parse output probe: %w", err)
	}

	durationSeconds, err := strconv.ParseFloat(probe.Format.Duration, 64)
	if err != nil {
		return fmt.Errorf("parse output duration %q: %w", probe.Format.Duration, err)
	}
	duration := time.Duration(durationSeconds * float64(time.Second))
	delta := duration - expected
	if delta < 0 {
		delta = -delta
	}
	if delta > 200*time.Millisecond {
		return fmt.Errorf("output duration %s differs from expected %s by %s", duration, expected, delta)
	}

	videoCount := 0
	audioCount := 0
	for _, stream := range probe.Streams {
		switch stream.CodecType {
		case "video":
			videoCount++
			if stream.CodecName != "h264" || stream.Width != 1080 || stream.Height != 1920 || stream.PixelFormat != "yuv420p" || stream.FrameRate != "30/1" {
				return fmt.Errorf("output video stream does not match H.264 1080x1920 yuv420p at 30/1")
			}
		case "audio":
			audioCount++
			if stream.CodecName != "aac" {
				return fmt.Errorf("output audio stream codec is %q, want AAC", stream.CodecName)
			}
		}
	}
	if videoCount != 1 {
		return fmt.Errorf("output has %d video streams, want 1", videoCount)
	}
	if audioCount != 1 {
		return fmt.Errorf("output has %d audio streams, want 1", audioCount)
	}
	return nil
}
