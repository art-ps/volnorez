package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"volnorez/internal/cli"
	"volnorez/internal/pipeline"
	"volnorez/internal/tools"
)

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--help"}, os.Getenv, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "volnorez INPUT") {
		t.Fatalf("stdout = %s", stdout.String())
	}
	for _, flagName := range []string{
		"--model", "--cover", "--output", "--language", "--start", "--duration",
		"--accent", "--title", "--font", "--whisper-bin", "--force", "--verbose",
	} {
		if !strings.Contains(stdout.String(), flagName) {
			t.Errorf("help missing %s", flagName)
		}
	}
}

func TestRunInvalidArgumentsUseExitTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, os.Getenv, &stdout, &stderr)
	if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "input") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunClassifiesParseErrors(t *testing.T) {
	tests := []struct {
		name string
		want int
		args func(*testing.T, string, string, string) []string
	}{
		{name: "missing input", want: 2, args: func(_ *testing.T, dir, _, model string) []string {
			return []string{filepath.Join(dir, "missing.mp3"), "--model", model}
		}},
		{name: "missing model option", want: 3, args: func(_ *testing.T, _, input, _ string) []string {
			return []string{input}
		}},
		{name: "missing model file", want: 3, args: func(_ *testing.T, dir, input, _ string) []string {
			return []string{input, "--model", filepath.Join(dir, "missing.bin")}
		}},
		{name: "invalid cover extension", want: 2, args: func(t *testing.T, dir, input, model string) []string {
			cover := filepath.Join(dir, "cover.gif")
			if err := os.WriteFile(cover, []byte("gif"), 0o600); err != nil {
				t.Fatal(err)
			}
			return []string{input, "--model", model, "--cover", cover}
		}},
		{name: "missing valid cover", want: 3, args: func(_ *testing.T, dir, input, model string) []string {
			return []string{input, "--model", model, "--cover", filepath.Join(dir, "missing.jpg")}
		}},
		{name: "invalid font extension", want: 2, args: func(t *testing.T, dir, input, model string) []string {
			font := filepath.Join(dir, "font.woff")
			if err := os.WriteFile(font, []byte("font"), 0o600); err != nil {
				t.Fatal(err)
			}
			return []string{input, "--model", model, "--font", font}
		}},
		{name: "missing valid font", want: 3, args: func(_ *testing.T, dir, input, model string) []string {
			return []string{input, "--model", model, "--font", filepath.Join(dir, "missing.otf")}
		}},
		{name: "missing explicit whisper", want: 3, args: func(_ *testing.T, dir, input, model string) []string {
			return []string{input, "--model", model, "--whisper-bin", filepath.Join(dir, "missing-whisper")}
		}},
		{name: "output collision", want: 2, args: func(t *testing.T, dir, input, model string) []string {
			output := filepath.Join(dir, "existing.mp4")
			if err := os.WriteFile(output, []byte("old"), 0o600); err != nil {
				t.Fatal(err)
			}
			return []string{input, "--model", model, "--output", output}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "episode.mp3")
			model := filepath.Join(dir, "model.bin")
			if err := os.WriteFile(input, []byte("mp3"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(model, []byte("model"), 0o600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			code := run(test.args(t, dir, input, model), func(string) string { return "" }, &stdout, &stderr)
			if code != test.want {
				t.Fatalf("code = %d, want %d; stderr = %q", code, test.want, stderr.String())
			}
			if stdout.Len() != 0 || strings.Count(stderr.String(), "\n") != 1 {
				t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunWithPrintsOnlyFinalAbsolutePathToStdout(t *testing.T) {
	args, output := validArgs(t)
	var stdout, stderr bytes.Buffer
	code := runWith(args, os.Getenv, &stdout, &stderr, appDeps{
		context: backgroundContext,
		pipelineDeps: func(tools.Runner) pipeline.Dependencies {
			return pipeline.Dependencies{}
		},
		pipeline: func(context.Context, cli.Config, pipeline.Dependencies, io.Writer) (string, error) {
			return output, nil
		},
	})
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.String() != output+"\n" {
		t.Fatalf("stdout = %q, want final path only", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunWithPrintsTersePipelineError(t *testing.T) {
	args, _ := validArgs(t)
	var stdout, stderr bytes.Buffer
	code := runWith(args, os.Getenv, &stdout, &stderr, appDeps{
		context: backgroundContext,
		pipelineDeps: func(tools.Runner) pipeline.Dependencies {
			return pipeline.Dependencies{}
		},
		pipeline: func(context.Context, cli.Config, pipeline.Dependencies, io.Writer) (string, error) {
			return "", errors.New("render failed")
		},
	})
	if code != 5 {
		t.Fatalf("code = %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.String() != "volnorez: render failed\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunWithVerboseStreamsChildDiagnosticsToStderr(t *testing.T) {
	args, _ := validArgs(t, "--verbose")
	var stdout, stderr bytes.Buffer
	code := runWith(args, os.Getenv, &stdout, &stderr, appDeps{
		context: backgroundContext,
		pipelineDeps: func(r tools.Runner) pipeline.Dependencies {
			return pipeline.Dependencies{Runner: r}
		},
		pipeline: func(ctx context.Context, _ cli.Config, deps pipeline.Dependencies, _ io.Writer) (string, error) {
			if _, err := deps.Runner.Run(ctx, tools.Command{
				Name: os.Args[0],
				Args: []string{"-test.run=TestMainHelperProcess", "--", "emit-no-newline", "exit"},
			}); err != nil {
				return "", fmt.Errorf("render failed: %w", err)
			}
			return "", nil
		},
	})
	if code != 5 {
		t.Fatalf("code = %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
	got := stderr.String()
	if strings.Count(got, "diagnostic without newline") != 1 {
		t.Fatalf("diagnostics duplicated: %q", got)
	}
	if !strings.Contains(got, `+ "`+os.Args[0]+`" "-test.run=TestMainHelperProcess" "--" "emit-no-newline" "exit"`+"\n") {
		t.Fatalf("missing quoted invocation: %q", got)
	}
	if !strings.Contains(got, "diagnostic without newline\nvolnorez: render failed: ") {
		t.Fatalf("diagnostic and final error collided: %q", got)
	}
	if strings.Count(got, "volnorez:") != 1 || !strings.HasSuffix(got, ": exit status 1\n") {
		t.Fatalf("final error = %q", got)
	}
}

func TestMainHelperProcess(t *testing.T) {
	for i, arg := range os.Args {
		if arg != "--" || i+1 >= len(os.Args) {
			continue
		}
		if os.Args[i+1] == "emit-no-newline" {
			_, _ = io.WriteString(os.Stderr, "diagnostic without newline")
		} else {
			_, _ = io.WriteString(os.Stderr, os.Args[i+1])
		}
		if i+2 < len(os.Args) && os.Args[i+2] == "exit" {
			os.Exit(1)
		}
		os.Exit(0)
	}
}

func TestRunWithMapsCanceledContextToExit130(t *testing.T) {
	args, _ := validArgs(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	code := runWith(args, os.Getenv, &stdout, &stderr, appDeps{
		context: func() (context.Context, context.CancelFunc) {
			return ctx, func() {}
		},
		pipelineDeps: func(tools.Runner) pipeline.Dependencies {
			return pipeline.Dependencies{}
		},
		pipeline: func(ctx context.Context, _ cli.Config, _ pipeline.Dependencies, _ io.Writer) (string, error) {
			return "", ctx.Err()
		},
	})
	if code != 130 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.String() != "volnorez: context canceled\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func backgroundContext() (context.Context, context.CancelFunc) {
	return context.Background(), func() {}
}

func validArgs(t *testing.T, extra ...string) ([]string, string) {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, "episode.mp3")
	model := filepath.Join(dir, "model.bin")
	output := filepath.Join(dir, "episode.mp4")
	for path, contents := range map[string]string{input: "mp3", model: "model"} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return append([]string{input, "--model", model, "--output", output}, extra...), output
}
