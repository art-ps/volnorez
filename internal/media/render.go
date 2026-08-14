package media

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
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
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		CodecType   string `json:"codec_type"`
		CodecName   string `json:"codec_name"`
		Width       int    `json:"width"`
		Height      int    `json:"height"`
		PixelFormat string `json:"pix_fmt"`
		FrameRate   string `json:"avg_frame_rate"`
		Channels    int    `json:"channels"`
		BitRate     string `json:"bit_rate"`
	} `json:"streams"`
}

func VerifyOutput(ctx context.Context, r tools.Runner, ffprobe, output string, expected time.Duration) error {
	result, err := r.Run(ctx, tools.Command{
		Name: ffprobe,
		Args: []string{
			"-v", "error",
			"-show_entries", "format=format_name,duration:stream=codec_type,codec_name,width,height,pix_fmt,avg_frame_rate,channels,bit_rate",
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
	if !hasFormat(probe.Format.FormatName, "mp4") {
		return fmt.Errorf("output container %q is not MP4", probe.Format.FormatName)
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
			if stream.Channels != 2 {
				return fmt.Errorf("output audio stream has %d channels, want stereo", stream.Channels)
			}
			bitRate, err := strconv.ParseInt(stream.BitRate, 10, 64)
			if err != nil {
				return fmt.Errorf("parse output audio bitrate %q: %w", stream.BitRate, err)
			}
			const targetBitRate int64 = 192000
			// ffprobe reports encoded average, which can approach zero for silence
			// even when the requested AAC target is 192k. Keep only an upper sanity
			// ceiling; BuildRenderCommand separately pins the encoder target to 192k.
			maximum := targetBitRate * 5 / 4
			if expected < 2*time.Second {
				maximum = targetBitRate * 3 / 2
			}
			if bitRate <= 0 || bitRate > maximum {
				return fmt.Errorf("output audio bitrate %d is outside 1..%d", bitRate, maximum)
			}
		}
	}
	if videoCount != 1 {
		return fmt.Errorf("output has %d video streams, want 1", videoCount)
	}
	if audioCount != 1 {
		return fmt.Errorf("output has %d audio streams, want 1", audioCount)
	}
	if err := verifyFaststart(output); err != nil {
		return err
	}
	return nil
}

func hasFormat(formats, want string) bool {
	for _, format := range strings.Split(formats, ",") {
		if format == want {
			return true
		}
	}
	return false
}

func verifyFaststart(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("inspect MP4 boxes: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect MP4 size: %w", err)
	}

	var moovOffset, mdatOffset int64 = -1, -1
	for offset := int64(0); offset < info.Size(); {
		remaining := info.Size() - offset
		if remaining < 8 {
			return fmt.Errorf("inspect MP4 boxes: truncated header at offset %d", offset)
		}
		var header [16]byte
		if _, err := file.ReadAt(header[:8], offset); err != nil {
			return fmt.Errorf("inspect MP4 box at offset %d: %w", offset, err)
		}
		headerSize := uint64(8)
		boxSize := uint64(binary.BigEndian.Uint32(header[:4]))
		if boxSize == 1 {
			if remaining < 16 {
				return fmt.Errorf("inspect MP4 boxes: truncated extended header at offset %d", offset)
			}
			if _, err := file.ReadAt(header[8:16], offset+8); err != nil {
				return fmt.Errorf("inspect MP4 extended box at offset %d: %w", offset, err)
			}
			headerSize = 16
			boxSize = binary.BigEndian.Uint64(header[8:16])
		} else if boxSize == 0 {
			boxSize = uint64(remaining)
		}
		if boxSize < headerSize || boxSize > uint64(remaining) {
			return fmt.Errorf("inspect MP4 boxes: invalid box size %d at offset %d", boxSize, offset)
		}
		switch string(header[4:8]) {
		case "moov":
			if moovOffset < 0 {
				moovOffset = offset
			}
		case "mdat":
			if mdatOffset < 0 {
				mdatOffset = offset
			}
		}
		offset += int64(boxSize)
	}
	if moovOffset < 0 || mdatOffset < 0 {
		return fmt.Errorf("output MP4 is missing moov or mdat box")
	}
	if moovOffset > mdatOffset {
		return fmt.Errorf("output MP4 is not faststart: moov follows mdat")
	}
	return nil
}
