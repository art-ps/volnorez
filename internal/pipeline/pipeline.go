package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"volnorez/internal/assets"
	"volnorez/internal/cli"
	"volnorez/internal/media"
	"volnorez/internal/subtitles"
	"volnorez/internal/tools"
	"volnorez/internal/transcribe"
)

type Dependencies struct {
	Runner     tools.Runner
	Preflight  func(context.Context, tools.Runner, string) (tools.Paths, error)
	Probe      func(context.Context, tools.Runner, string, string) (media.InputInfo, error)
	Transcribe func(context.Context, tools.Runner, transcribe.Request) ([]transcribe.Word, error)
	Render     func(context.Context, tools.Runner, media.RenderRequest) error
	Verify     func(context.Context, tools.Runner, string, string, time.Duration) error
	TempParent string
}

func DefaultDependencies(r tools.Runner) Dependencies {
	return Dependencies{
		Runner:     r,
		Preflight:  tools.Preflight,
		Probe:      media.Probe,
		Transcribe: transcribe.Run,
		Render:     media.Render,
		Verify:     media.VerifyOutput,
	}
}

func Run(ctx context.Context, cfg cli.Config, deps Dependencies, progress io.Writer) (string, error) {
	fmt.Fprintln(progress, "checking input")
	paths, err := deps.Preflight(ctx, deps.Runner, cfg.WhisperBin)
	if err != nil {
		return "", &Error{Code: 3, Op: "checking tools", Err: err}
	}
	info, err := deps.Probe(ctx, deps.Runner, paths.FFprobe, cfg.Input)
	if err != nil {
		return "", &Error{Code: 2, Op: "reading input", Err: err}
	}
	selection, err := media.SelectInterval(info, cfg.Start, cfg.Duration, cfg.HasDuration)
	if err != nil {
		return "", &Error{Code: 2, Op: "selecting interval", Err: err}
	}
	workspace, err := NewWorkspace(deps.TempParent)
	if err != nil {
		return "", &Error{Code: 5, Op: "creating workspace", Err: err}
	}
	defer workspace.Cleanup()
	fontDir, fontFamily, err := assets.PrepareFont(workspace.Dir, cfg.Font)
	if err != nil {
		return "", &Error{Code: 3, Op: "preparing font", Err: err}
	}
	wavPath := filepath.Join(workspace.Dir, "speech.wav")
	coverPath := filepath.Join(workspace.Dir, "cover.png")
	if err := media.ExtractAudio(ctx, deps.Runner, paths.FFmpeg, cfg.Input, wavPath, selection); err != nil {
		return "", &Error{Code: 5, Op: "extracting audio", Err: err}
	}
	if err := media.PrepareCover(ctx, deps.Runner, paths.FFmpeg, cfg.Input, cfg.Cover, coverPath, info.CoverStream); err != nil {
		return "", &Error{Code: 3, Op: "preparing cover", Err: err}
	}

	fmt.Fprintln(progress, "transcribing")
	prefix := filepath.Join(workspace.Dir, "transcript")
	words, err := deps.Transcribe(ctx, deps.Runner, transcribe.Request{
		Whisper: paths.Whisper, Model: cfg.Model, WAV: wavPath, Language: cfg.Language,
		OutputPrefix: prefix, Duration: selection.Duration,
	})
	if err != nil {
		return "", &Error{Code: 4, Op: "transcribing", Err: err}
	}
	title := cfg.Title
	if title == "" {
		title = info.Title
	}
	assPath := filepath.Join(workspace.Dir, "subtitles.ass")
	assFile, err := os.Create(assPath)
	if err != nil {
		return "", &Error{Code: 5, Op: "creating subtitles", Err: err}
	}
	writeErr := subtitles.WriteASS(assFile, subtitles.Document{
		Duration: selection.Duration, Title: title, FontFamily: fontFamily,
		Accent: cfg.Accent, Phrases: subtitles.Group(words),
	})
	closeErr := assFile.Close()
	if writeErr != nil {
		return "", &Error{Code: 5, Op: "writing subtitles", Err: writeErr}
	}
	if closeErr != nil {
		return "", &Error{Code: 5, Op: "closing subtitles", Err: closeErr}
	}

	if !cfg.Force {
		if _, err := os.Stat(cfg.Output); err == nil {
			return "", &Error{Code: 2, Op: "publishing output", Err: fmt.Errorf("%s exists; use --force", cfg.Output)}
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", &Error{Code: 5, Op: "checking output", Err: err}
		}
	}
	tempFile, err := os.CreateTemp(filepath.Dir(cfg.Output), "."+filepath.Base(cfg.Output)+".volnorez-*.tmp.mp4")
	if err != nil {
		return "", &Error{Code: 5, Op: "creating output", Err: err}
	}
	tempOutput := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempOutput)
		return "", &Error{Code: 5, Op: "closing output", Err: err}
	}
	defer os.Remove(tempOutput)
	fmt.Fprintln(progress, "rendering")
	err = deps.Render(ctx, deps.Runner, media.RenderRequest{
		FFmpeg: paths.FFmpeg, Input: cfg.Input, Cover: coverPath, ASS: assPath,
		FontDir: fontDir, WorkDir: workspace.Dir, Output: tempOutput,
		Accent: cfg.Accent, Selection: selection,
	})
	if err != nil {
		return "", &Error{Code: 5, Op: "rendering", Err: err}
	}
	if err := deps.Verify(ctx, deps.Runner, paths.FFprobe, tempOutput, selection.Duration); err != nil {
		return "", &Error{Code: 5, Op: "verifying output", Err: err}
	}
	if !cfg.Force {
		if _, err := os.Stat(cfg.Output); err == nil {
			return "", &Error{Code: 2, Op: "publishing output", Err: fmt.Errorf("%s appeared during rendering", cfg.Output)}
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", &Error{Code: 5, Op: "checking output", Err: err}
		}
	}
	if cfg.Force {
		if err := os.Rename(tempOutput, cfg.Output); err != nil {
			return "", &Error{Code: 5, Op: "publishing output", Err: err}
		}
	} else {
		if err := os.Link(tempOutput, cfg.Output); err != nil {
			code := 5
			if errors.Is(err, os.ErrExist) {
				code = 2
			}
			return "", &Error{Code: code, Op: "publishing output", Err: err}
		}
	}
	return cfg.Output, nil
}
