package main

import (
	"bytes"
	"context"
	"errors"
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
			if _, err := deps.Runner.Run(ctx, tools.Command{Name: "sh", Args: []string{"-c", "printf diagnostic >&2"}}); err != nil {
				return "", err
			}
			return "", errors.New("render failed")
		},
	})
	if code != 5 {
		t.Fatalf("code = %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.String() != "diagnosticvolnorez: render failed\n" {
		t.Fatalf("stderr = %q", stderr.String())
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
