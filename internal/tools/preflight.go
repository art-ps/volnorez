package tools

import (
	"context"
	"fmt"
	"regexp"
)

type Paths struct {
	FFmpeg  string
	FFprobe string
	Whisper string
}

func Preflight(ctx context.Context, r Runner, whisperOverride string) (Paths, error) {
	ffmpeg, err := lookPath(r, "ffmpeg")
	if err != nil {
		return Paths{}, err
	}
	ffprobe, err := lookPath(r, "ffprobe")
	if err != nil {
		return Paths{}, err
	}
	whisper := whisperOverride
	if whisper == "" {
		whisper, err = lookPath(r, "whisper-cli")
		if err != nil {
			return Paths{}, err
		}
	}

	if err := requireCapabilities(ctx, r, Command{Name: whisper, Args: []string{"--help"}}, []string{"--output-json-full", "--max-len", "--language"}); err != nil {
		return Paths{}, fmt.Errorf("check whisper-cli: %w", err)
	}
	if err := requireCapabilities(ctx, r, Command{Name: ffmpeg, Args: []string{"-hide_banner", "-filters"}}, []string{"showwaves", "subtitles", "geq"}); err != nil {
		return Paths{}, fmt.Errorf("check ffmpeg filters: %w", err)
	}
	if err := requireCapabilities(ctx, r, Command{Name: ffmpeg, Args: []string{"-hide_banner", "-encoders"}}, []string{"libx264", "aac"}); err != nil {
		return Paths{}, fmt.Errorf("check ffmpeg encoders: %w", err)
	}
	return Paths{FFmpeg: ffmpeg, FFprobe: ffprobe, Whisper: whisper}, nil
}

func lookPath(r Runner, name string) (string, error) {
	path, err := r.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("find %s: %w", name, err)
	}
	return path, nil
}

func requireCapabilities(ctx context.Context, r Runner, command Command, required []string) error {
	result, err := r.Run(ctx, command)
	if err != nil {
		return err
	}
	output := string(result.Stdout) + "\n" + string(result.Stderr)
	for _, capability := range required {
		if !hasToken(output, capability) {
			return fmt.Errorf("missing capability %s%s", capability, capabilityHint(capability))
		}
	}
	return nil
}

func capabilityHint(capability string) string {
	if capability == "subtitles" {
		return " (ffmpeg built without libass; on macOS run: brew unlink ffmpeg && brew link --force ffmpeg-full)"
	}
	return ""
}

func hasToken(output, token string) bool {
	return regexp.MustCompile(`(^|[^A-Za-z0-9_-])` + regexp.QuoteMeta(token) + `($|[^A-Za-z0-9_-])`).MatchString(output)
}
