package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type fakeRunner struct {
	look map[string]string
	run  func(context.Context, Command) (Result, error)
}

func (f fakeRunner) LookPath(name string) (string, error) {
	path, ok := f.look[name]
	if !ok {
		return "", fmt.Errorf("%s not found", name)
	}
	return path, nil
}

func (f fakeRunner) Run(ctx context.Context, command Command) (Result, error) {
	return f.run(ctx, command)
}

func TestPreflightAcceptsRequiredCapabilities(t *testing.T) {
	f := capableRunner()
	paths, err := Preflight(context.Background(), f, "")
	if err != nil {
		t.Fatal(err)
	}
	if paths != (Paths{FFmpeg: "/bin/ffmpeg", FFprobe: "/bin/ffprobe", Whisper: "/bin/whisper-cli"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestPreflightAcceptsCapabilitiesFromStderr(t *testing.T) {
	f := capableRunner()
	f.run = func(_ context.Context, c Command) (Result, error) {
		switch strings.Join(c.Args, " ") {
		case "--help":
			return Result{Stderr: []byte("--output-json-full --max-len --language")}, nil
		case "-hide_banner -filters", "-hide_banner -encoders":
			return Result{Stderr: []byte(capabilityOutput(strings.Join(c.Args, " ")))}, nil
		default:
			return Result{}, fmt.Errorf("unexpected command: %#v", c)
		}
	}
	if _, err := Preflight(context.Background(), f, ""); err != nil {
		t.Fatal(err)
	}
}

func TestPreflightRejectsMissingExecutable(t *testing.T) {
	for _, missing := range []string{"ffmpeg", "ffprobe", "whisper-cli"} {
		t.Run(missing, func(t *testing.T) {
			f := capableRunner()
			delete(f.look, missing)
			_, err := Preflight(context.Background(), f, "")
			if err == nil || !strings.Contains(err.Error(), missing) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestPreflightRejectsMissingWhisperFlag(t *testing.T) {
	for _, flag := range []string{"--output-json-full", "--max-len", "--language"} {
		t.Run(flag, func(t *testing.T) {
			f := capableRunner()
			f.run = func(_ context.Context, c Command) (Result, error) {
				if strings.Join(c.Args, " ") == "--help" {
					return Result{Stderr: []byte(strings.ReplaceAll("--output-json-full --max-len --language", flag, ""))}, nil
				}
				return supportedFFmpeg(c)
			}
			_, err := Preflight(context.Background(), f, "")
			if err == nil || !strings.Contains(err.Error(), flag) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestPreflightRejectsMissingFFmpegCapability(t *testing.T) {
	tests := []struct {
		name string
		args string
	}{
		{"filter showwaves", "-hide_banner -filters"},
		{"filter subtitles", "-hide_banner -filters"},
		{"filter geq", "-hide_banner -filters"},
		{"encoder libx264", "-hide_banner -encoders"},
		{"encoder aac", "-hide_banner -encoders"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := capableRunner()
			missing := strings.TrimPrefix(test.name, "filter ")
			missing = strings.TrimPrefix(missing, "encoder ")
			f.run = func(_ context.Context, c Command) (Result, error) {
				if strings.Join(c.Args, " ") == test.args {
					return Result{Stdout: []byte(strings.ReplaceAll(capabilityOutput(test.args), missing, ""))}, nil
				}
				if strings.Join(c.Args, " ") == "--help" {
					return Result{Stdout: []byte("--output-json-full --max-len --language")}, nil
				}
				return supportedFFmpeg(c)
			}
			_, err := Preflight(context.Background(), f, "")
			if err == nil || !strings.Contains(err.Error(), missing) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestPreflightUsesExplicitWhisperBin(t *testing.T) {
	f := capableRunner()
	delete(f.look, "whisper-cli")
	var whisperCommand Command
	f.run = func(_ context.Context, c Command) (Result, error) {
		if strings.Join(c.Args, " ") == "--help" {
			whisperCommand = c
			return Result{Stdout: []byte("--output-json-full --max-len --language")}, nil
		}
		return supportedFFmpeg(c)
	}
	paths, err := Preflight(context.Background(), f, "/custom/whisper-cli")
	if err != nil {
		t.Fatal(err)
	}
	if paths.Whisper != "/custom/whisper-cli" || whisperCommand.Name != "/custom/whisper-cli" {
		t.Fatalf("paths = %#v, command = %#v", paths, whisperCommand)
	}
}

func capableRunner() fakeRunner {
	return fakeRunner{
		look: map[string]string{"ffmpeg": "/bin/ffmpeg", "ffprobe": "/bin/ffprobe", "whisper-cli": "/bin/whisper-cli"},
		run: func(_ context.Context, c Command) (Result, error) {
			if strings.Join(c.Args, " ") == "--help" {
				return Result{Stdout: []byte("--output-json-full --max-len --language")}, nil
			}
			return supportedFFmpeg(c)
		},
	}
}

func supportedFFmpeg(c Command) (Result, error) {
	output := capabilityOutput(strings.Join(c.Args, " "))
	if output == "" {
		return Result{}, fmt.Errorf("unexpected command: %#v", c)
	}
	return Result{Stdout: []byte(output)}, nil
}

func capabilityOutput(args string) string {
	switch args {
	case "-hide_banner -filters":
		return "showwaves subtitles geq"
	case "-hide_banner -encoders":
		return "libx264 aac"
	default:
		return ""
	}
}
