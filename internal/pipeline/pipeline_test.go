package pipeline

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"volnorez/internal/cli"
	"volnorez/internal/media"
	"volnorez/internal/tools"
	"volnorez/internal/transcribe"
)

type pipelineRunner struct {
	t      *testing.T
	calls  *[]string
	failAt string
	onRun  func(tools.Command)
}

func (r *pipelineRunner) LookPath(string) (string, error) {
	r.t.Helper()
	return "", errors.New("unexpected LookPath call")
}

func (r *pipelineRunner) Run(_ context.Context, command tools.Command) (tools.Result, error) {
	r.t.Helper()
	if r.onRun != nil {
		r.onRun(command)
	}
	if len(command.Args) == 0 {
		return tools.Result{}, errors.New("unexpected empty command")
	}
	output := command.Args[len(command.Args)-1]
	stage := ""
	switch filepath.Base(output) {
	case "speech.wav":
		stage = "extract"
	case "cover.png":
		stage = "cover"
	default:
		return tools.Result{}, fmt.Errorf("unexpected runner command: %#v", command)
	}
	*r.calls = append(*r.calls, stage)
	if r.failAt == stage {
		return tools.Result{}, errors.New(stage + " failed")
	}
	if err := os.WriteFile(output, []byte(stage), 0o600); err != nil {
		r.t.Fatal(err)
	}
	return tools.Result{}, nil
}

func TestRunOrchestratesInOrderAndPublishesOutput(t *testing.T) {
	root := t.TempDir()
	workspaceParent := filepath.Join(root, "workspaces")
	if err := os.Mkdir(workspaceParent, 0o700); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "clip.mp4")
	explicitCover := filepath.Join(root, "cover.jpg")
	calls := []string{}
	runner := &pipelineRunner{t: t, calls: &calls}
	deps := successfulDependencies(t, runner, workspaceParent, &calls)
	deps.Probe = func(context.Context, tools.Runner, string, string) (media.InputInfo, error) {
		calls = append(calls, "probe")
		return media.InputInfo{Duration: 10 * time.Second, HasAudio: true, Title: "metadata title", CoverStream: 4}, nil
	}
	runner.onRun = func(command tools.Command) {
		if filepath.Base(command.Args[len(command.Args)-1]) != "cover.png" {
			return
		}
		if !containsAdjacent(command.Args, "-i", explicitCover) {
			t.Errorf("cover command does not use explicit cover: %#v", command.Args)
		}
		if containsAdjacent(command.Args, "-map", "0:4") {
			t.Errorf("explicit cover must take precedence over embedded cover: %#v", command.Args)
		}
	}
	deps.Transcribe = func(_ context.Context, _ tools.Runner, request transcribe.Request) ([]transcribe.Word, error) {
		calls = append(calls, "transcribe")
		if request.Duration != 9*time.Second || request.WAV != filepath.Join(filepath.Dir(request.OutputPrefix), "speech.wav") {
			t.Errorf("unexpected transcription request: %#v", request)
		}
		return []transcribe.Word{{Text: "hello", Start: 0, End: time.Second}}, nil
	}
	deps.Render = func(_ context.Context, _ tools.Runner, request media.RenderRequest) error {
		calls = append(calls, "render")
		if filepath.Dir(request.Output) != filepath.Dir(output) {
			t.Errorf("temporary output must be sibling of destination: %q", request.Output)
		}
		if !strings.HasPrefix(filepath.Base(request.Output), ".clip.mp4.volnorez-") {
			t.Errorf("unexpected temporary output name: %q", request.Output)
		}
		ass, err := os.ReadFile(request.ASS)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(ass, []byte("CLI title")) || bytes.Contains(ass, []byte("metadata title")) {
			t.Errorf("CLI title must take precedence, ASS was:\n%s", ass)
		}
		return os.WriteFile(request.Output, []byte("rendered"), 0o600)
	}
	deps.Verify = func(_ context.Context, _ tools.Runner, _ string, path string, duration time.Duration) error {
		calls = append(calls, "verify")
		if duration != 9*time.Second {
			t.Errorf("verification duration = %s, want 9s", duration)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "rendered" {
			t.Errorf("verification read %q", data)
		}
		if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("destination published before verification: %v", err)
		}
		return nil
	}

	cfg := cli.Config{
		Input: "input.mp3", Model: "model.bin", Cover: explicitCover, Output: output,
		Language: "auto", Accent: "#112233", Title: "CLI title", Start: time.Second,
	}
	var progress bytes.Buffer
	result, err := Run(context.Background(), cfg, deps, &progress)
	if err != nil {
		t.Fatal(err)
	}
	if result != output {
		t.Fatalf("result = %q, want %q", result, output)
	}
	if progress.String() != "checking input\ntranscribing\nrendering\n" {
		t.Fatalf("progress = %q", progress.String())
	}
	wantCalls := []string{"preflight", "probe", "extract", "cover", "transcribe", "render", "verify"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
	}
	assertFileContents(t, output, "rendered")
	assertCleaned(t, workspaceParent, output)
}

func TestRunUsesEmbeddedCoverAndMetadataTitleAsFallbacks(t *testing.T) {
	root := t.TempDir()
	workspaceParent := filepath.Join(root, "workspaces")
	if err := os.Mkdir(workspaceParent, 0o700); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "clip.mp4")
	calls := []string{}
	runner := &pipelineRunner{t: t, calls: &calls}
	deps := successfulDependencies(t, runner, workspaceParent, &calls)
	runner.onRun = func(command tools.Command) {
		if filepath.Base(command.Args[len(command.Args)-1]) == "cover.png" && !containsAdjacent(command.Args, "-map", "0:4") {
			t.Errorf("embedded cover stream not selected: %#v", command.Args)
		}
	}
	deps.Render = func(_ context.Context, _ tools.Runner, request media.RenderRequest) error {
		calls = append(calls, "render")
		ass, err := os.ReadFile(request.ASS)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(ass, []byte("metadata title")) {
			t.Errorf("metadata title missing from ASS:\n%s", ass)
		}
		return os.WriteFile(request.Output, []byte("rendered"), 0o600)
	}

	_, err := Run(context.Background(), cli.Config{
		Input: "input.mp3", Model: "model.bin", Output: output, Language: "auto", Accent: "#112233",
	}, deps, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	assertCleaned(t, workspaceParent, output)
}

func TestRunWrapsFailuresAndCleansTemporaryFiles(t *testing.T) {
	tests := []struct {
		name string
		code int
		op   string
		set  func(*testing.T, *cli.Config, *Dependencies, *pipelineRunner)
	}{
		{
			name: "preflight", code: 3, op: "checking tools",
			set: func(_ *testing.T, _ *cli.Config, deps *Dependencies, _ *pipelineRunner) {
				deps.Preflight = func(context.Context, tools.Runner, string) (tools.Paths, error) {
					return tools.Paths{}, errors.New("preflight failed")
				}
			},
		},
		{
			name: "probe", code: 2, op: "reading input",
			set: func(_ *testing.T, _ *cli.Config, deps *Dependencies, _ *pipelineRunner) {
				deps.Probe = func(context.Context, tools.Runner, string, string) (media.InputInfo, error) {
					return media.InputInfo{}, errors.New("probe failed")
				}
			},
		},
		{
			name: "interval", code: 2, op: "selecting interval",
			set: func(_ *testing.T, cfg *cli.Config, _ *Dependencies, _ *pipelineRunner) {
				cfg.Start = 20 * time.Second
			},
		},
		{
			name: "font", code: 3, op: "preparing font",
			set: func(_ *testing.T, cfg *cli.Config, _ *Dependencies, _ *pipelineRunner) {
				cfg.Font = filepath.Join(filepath.Dir(cfg.Output), "missing.ttf")
			},
		},
		{
			name: "extract", code: 5, op: "extracting audio",
			set: func(_ *testing.T, _ *cli.Config, _ *Dependencies, runner *pipelineRunner) {
				runner.failAt = "extract"
			},
		},
		{
			name: "cover", code: 3, op: "preparing cover",
			set: func(_ *testing.T, _ *cli.Config, _ *Dependencies, runner *pipelineRunner) {
				runner.failAt = "cover"
			},
		},
		{
			name: "transcribe", code: 4, op: "transcribing",
			set: func(_ *testing.T, _ *cli.Config, deps *Dependencies, _ *pipelineRunner) {
				deps.Transcribe = func(context.Context, tools.Runner, transcribe.Request) ([]transcribe.Word, error) {
					return nil, errors.New("transcribe failed")
				}
			},
		},
		{
			name: "subtitles", code: 5, op: "creating subtitles",
			set: func(t *testing.T, _ *cli.Config, deps *Dependencies, _ *pipelineRunner) {
				deps.Transcribe = func(_ context.Context, _ tools.Runner, request transcribe.Request) ([]transcribe.Word, error) {
					if err := os.Mkdir(filepath.Join(filepath.Dir(request.OutputPrefix), "subtitles.ass"), 0o700); err != nil {
						t.Fatal(err)
					}
					return []transcribe.Word{{Text: "hello", Start: 0, End: time.Second}}, nil
				}
			},
		},
		{
			name: "render", code: 5, op: "rendering",
			set: func(_ *testing.T, _ *cli.Config, deps *Dependencies, _ *pipelineRunner) {
				deps.Render = func(_ context.Context, _ tools.Runner, request media.RenderRequest) error {
					if err := os.WriteFile(request.Output, []byte("partial"), 0o600); err != nil {
						return err
					}
					return errors.New("render failed")
				}
			},
		},
		{
			name: "verify", code: 5, op: "verifying output",
			set: func(_ *testing.T, _ *cli.Config, deps *Dependencies, _ *pipelineRunner) {
				deps.Verify = func(context.Context, tools.Runner, string, string, time.Duration) error {
					return errors.New("verify failed")
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			workspaceParent := filepath.Join(root, "workspaces")
			if err := os.Mkdir(workspaceParent, 0o700); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(root, "clip.mp4")
			calls := []string{}
			runner := &pipelineRunner{t: t, calls: &calls}
			deps := successfulDependencies(t, runner, workspaceParent, &calls)
			cfg := cli.Config{Input: "input.mp3", Model: "model.bin", Output: output, Language: "auto", Accent: "#112233"}
			test.set(t, &cfg, &deps, runner)

			_, err := Run(context.Background(), cfg, deps, &bytes.Buffer{})
			if err == nil {
				t.Fatal("Run() error = nil")
			}
			if Code(err) != test.code {
				t.Fatalf("Code(error) = %d, want %d (%v)", Code(err), test.code, err)
			}
			var pipelineErr *Error
			if !errors.As(err, &pipelineErr) || pipelineErr.Op != test.op {
				t.Fatalf("error = %v, want pipeline Error op %q", err, test.op)
			}
			assertCleaned(t, workspaceParent, output)
		})
	}
}

func TestRunCancellationHasExitCode130(t *testing.T) {
	root := t.TempDir()
	workspaceParent := filepath.Join(root, "workspaces")
	if err := os.Mkdir(workspaceParent, 0o700); err != nil {
		t.Fatal(err)
	}
	calls := []string{}
	runner := &pipelineRunner{t: t, calls: &calls}
	deps := successfulDependencies(t, runner, workspaceParent, &calls)
	deps.Transcribe = func(context.Context, tools.Runner, transcribe.Request) ([]transcribe.Word, error) {
		return nil, context.Canceled
	}
	output := filepath.Join(root, "clip.mp4")

	_, err := Run(context.Background(), cli.Config{
		Input: "input.mp3", Model: "model.bin", Output: output, Language: "auto", Accent: "#112233",
	}, deps, &bytes.Buffer{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if Code(err) != 130 {
		t.Fatalf("Code(error) = %d, want 130", Code(err))
	}
	assertCleaned(t, workspaceParent, output)
}

func TestRunLateCancellationCleansTemporaryFilesAndPreservesForcedOutput(t *testing.T) {
	root := t.TempDir()
	workspaceParent := filepath.Join(root, "workspaces")
	if err := os.Mkdir(workspaceParent, 0o700); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "clip.mp4")
	if err := os.WriteFile(output, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := []string{}
	runner := &pipelineRunner{t: t, calls: &calls}
	deps := successfulDependencies(t, runner, workspaceParent, &calls)
	ctx, cancel := context.WithCancel(context.Background())
	deps.Verify = func(ctx context.Context, _ tools.Runner, _ string, tempOutput string, _ time.Duration) error {
		assertFileContents(t, tempOutput, "rendered")
		entries, err := os.ReadDir(workspaceParent)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("workspace count during verification = %d, want 1", len(entries))
		}
		cancel()
		return ctx.Err()
	}

	_, err := Run(ctx, cli.Config{
		Input: "input.mp3", Model: "model.bin", Output: output, Language: "auto", Accent: "#112233", Force: true,
	}, deps, &bytes.Buffer{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if Code(err) != 130 {
		t.Fatalf("Code(error) = %d, want 130", Code(err))
	}
	assertFileContents(t, output, "old")
	assertCleaned(t, workspaceParent, output)
}

func TestRunDoesNotPublishWhenVerifyCancelsThenReturnsNil(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%t", force), func(t *testing.T) {
			root := t.TempDir()
			workspaceParent := filepath.Join(root, "workspaces")
			if err := os.Mkdir(workspaceParent, 0o700); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(root, "clip.mp4")
			if force {
				if err := os.WriteFile(output, []byte("old"), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			calls := []string{}
			runner := &pipelineRunner{t: t, calls: &calls}
			deps := successfulDependencies(t, runner, workspaceParent, &calls)
			ctx, cancel := context.WithCancel(context.Background())
			deps.Verify = func(context.Context, tools.Runner, string, string, time.Duration) error {
				cancel()
				return nil
			}
			deps.Link = func(string, string) error {
				t.Fatal("Link called after verification canceled the context")
				return nil
			}
			deps.Rename = func(string, string) error {
				t.Fatal("Rename called after verification canceled the context")
				return nil
			}

			_, err := Run(ctx, cli.Config{
				Input: "input.mp3", Model: "model.bin", Output: output, Language: "auto", Accent: "#112233", Force: force,
			}, deps, &bytes.Buffer{})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Run() error = %v, want context.Canceled", err)
			}
			if Code(err) != 130 {
				t.Fatalf("Code(error) = %d, want 130", Code(err))
			}
			if force {
				assertFileContents(t, output, "old")
			} else if _, statErr := os.Stat(output); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("destination exists after cancellation: %v", statErr)
			}
			assertCleaned(t, workspaceParent, output)
		})
	}
}

func TestRunWorkspaceCreationFailure(t *testing.T) {
	root := t.TempDir()
	missingParent := filepath.Join(root, "missing", "workspaces")
	calls := []string{}
	runner := &pipelineRunner{t: t, calls: &calls}
	deps := successfulDependencies(t, runner, missingParent, &calls)
	output := filepath.Join(root, "clip.mp4")

	_, err := Run(context.Background(), cli.Config{
		Input: "input.mp3", Model: "model.bin", Output: output, Language: "auto", Accent: "#112233",
	}, deps, &bytes.Buffer{})
	if Code(err) != 5 {
		t.Fatalf("Code(error) = %d, want 5 (%v)", Code(err), err)
	}
	var pipelineErr *Error
	if !errors.As(err, &pipelineErr) || pipelineErr.Op != "creating workspace" {
		t.Fatalf("error = %v, want pipeline Error op %q", err, "creating workspace")
	}
	if _, statErr := os.Stat(missingParent); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("failed workspace creation left path behind: %v", statErr)
	}
	if _, statErr := os.Stat(output); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("failed workspace creation changed output: %v", statErr)
	}
}

func TestRunSiblingTemporaryOutputCreationFailure(t *testing.T) {
	root := t.TempDir()
	workspaceParent := filepath.Join(root, "workspaces")
	if err := os.Mkdir(workspaceParent, 0o700); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "clip.mp4")
	calls := []string{}
	runner := &pipelineRunner{t: t, calls: &calls}
	deps := successfulDependencies(t, runner, workspaceParent, &calls)
	deps.CreateTemp = func(dir, pattern string) (*os.File, error) {
		if dir != root || pattern != ".clip.mp4.volnorez-*.tmp.mp4" {
			t.Errorf("CreateTemp(%q, %q), want sibling output pattern", dir, pattern)
		}
		return nil, errors.New("create temp failed")
	}
	deps.Render = func(context.Context, tools.Runner, media.RenderRequest) error {
		t.Fatal("render called after sibling temporary-output creation failure")
		return nil
	}

	_, err := Run(context.Background(), cli.Config{
		Input: "input.mp3", Model: "model.bin", Output: output, Language: "auto", Accent: "#112233",
	}, deps, &bytes.Buffer{})
	if Code(err) != 5 {
		t.Fatalf("Code(error) = %d, want 5 (%v)", Code(err), err)
	}
	var pipelineErr *Error
	if !errors.As(err, &pipelineErr) || pipelineErr.Op != "creating output" {
		t.Fatalf("error = %v, want pipeline Error op %q", err, "creating output")
	}
	assertCleaned(t, workspaceParent, output)
}

func TestRunForcePreservesOldOutputOnFailureAndReplacesAfterVerification(t *testing.T) {
	for _, stage := range []string{"render", "verify"} {
		t.Run("failure_"+stage, func(t *testing.T) {
			root := t.TempDir()
			workspaceParent := filepath.Join(root, "workspaces")
			if err := os.Mkdir(workspaceParent, 0o700); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(root, "clip.mp4")
			if err := os.WriteFile(output, []byte("old"), 0o600); err != nil {
				t.Fatal(err)
			}
			calls := []string{}
			runner := &pipelineRunner{t: t, calls: &calls}
			deps := successfulDependencies(t, runner, workspaceParent, &calls)
			if stage == "render" {
				deps.Render = func(_ context.Context, _ tools.Runner, request media.RenderRequest) error {
					if err := os.WriteFile(request.Output, []byte("partial"), 0o600); err != nil {
						t.Fatal(err)
					}
					return errors.New("render failed")
				}
			} else {
				deps.Verify = func(context.Context, tools.Runner, string, string, time.Duration) error {
					return errors.New("verify failed")
				}
			}

			_, err := Run(context.Background(), cli.Config{
				Input: "input.mp3", Model: "model.bin", Output: output, Language: "auto", Accent: "#112233", Force: true,
			}, deps, &bytes.Buffer{})
			if err == nil {
				t.Fatal("Run() error = nil")
			}
			assertFileContents(t, output, "old")
			assertCleaned(t, workspaceParent, output)
		})
	}

	t.Run("success", func(t *testing.T) {
		root := t.TempDir()
		workspaceParent := filepath.Join(root, "workspaces")
		if err := os.Mkdir(workspaceParent, 0o700); err != nil {
			t.Fatal(err)
		}
		output := filepath.Join(root, "clip.mp4")
		if err := os.WriteFile(output, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
		calls := []string{}
		runner := &pipelineRunner{t: t, calls: &calls}
		deps := successfulDependencies(t, runner, workspaceParent, &calls)
		verified := false
		deps.Verify = func(_ context.Context, _ tools.Runner, _ string, tempOutput string, _ time.Duration) error {
			assertFileContents(t, output, "old")
			assertFileContents(t, tempOutput, "rendered")
			verified = true
			return nil
		}

		_, err := Run(context.Background(), cli.Config{
			Input: "input.mp3", Model: "model.bin", Output: output, Language: "auto", Accent: "#112233", Force: true,
		}, deps, &bytes.Buffer{})
		if err != nil {
			t.Fatal(err)
		}
		if !verified {
			t.Fatal("output was replaced without verification")
		}
		assertFileContents(t, output, "rendered")
		assertCleaned(t, workspaceParent, output)
	})
}

func TestRunWithoutForceNeverClobbersOutput(t *testing.T) {
	t.Run("existing destination is rejected before render", func(t *testing.T) {
		root := t.TempDir()
		workspaceParent := filepath.Join(root, "workspaces")
		if err := os.Mkdir(workspaceParent, 0o700); err != nil {
			t.Fatal(err)
		}
		output := filepath.Join(root, "clip.mp4")
		if err := os.WriteFile(output, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
		calls := []string{}
		runner := &pipelineRunner{t: t, calls: &calls}
		deps := successfulDependencies(t, runner, workspaceParent, &calls)
		deps.Render = func(context.Context, tools.Runner, media.RenderRequest) error {
			t.Fatal("render called for existing destination")
			return nil
		}

		_, err := Run(context.Background(), cli.Config{
			Input: "input.mp3", Model: "model.bin", Output: output, Language: "auto", Accent: "#112233",
		}, deps, &bytes.Buffer{})
		if Code(err) != 2 {
			t.Fatalf("Code(error) = %d, want 2 (%v)", Code(err), err)
		}
		assertFileContents(t, output, "old")
		assertCleaned(t, workspaceParent, output)
	})

	t.Run("destination appearing during render is preserved", func(t *testing.T) {
		root := t.TempDir()
		workspaceParent := filepath.Join(root, "workspaces")
		if err := os.Mkdir(workspaceParent, 0o700); err != nil {
			t.Fatal(err)
		}
		output := filepath.Join(root, "clip.mp4")
		calls := []string{}
		runner := &pipelineRunner{t: t, calls: &calls}
		deps := successfulDependencies(t, runner, workspaceParent, &calls)
		deps.Render = func(_ context.Context, _ tools.Runner, request media.RenderRequest) error {
			if err := os.WriteFile(request.Output, []byte("rendered"), 0o600); err != nil {
				t.Fatal(err)
			}
			return os.WriteFile(output, []byte("racer"), 0o600)
		}

		_, err := Run(context.Background(), cli.Config{
			Input: "input.mp3", Model: "model.bin", Output: output, Language: "auto", Accent: "#112233",
		}, deps, &bytes.Buffer{})
		if Code(err) != 2 {
			t.Fatalf("Code(error) = %d, want 2 (%v)", Code(err), err)
		}
		assertFileContents(t, output, "racer")
		assertCleaned(t, workspaceParent, output)
	})
}

func TestRunLinkFailurePreservesDestinationAndCleansTemporaryFiles(t *testing.T) {
	root := t.TempDir()
	workspaceParent := filepath.Join(root, "workspaces")
	if err := os.Mkdir(workspaceParent, 0o700); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "clip.mp4")
	calls := []string{}
	runner := &pipelineRunner{t: t, calls: &calls}
	deps := successfulDependencies(t, runner, workspaceParent, &calls)
	verified := false
	deps.Verify = func(context.Context, tools.Runner, string, string, time.Duration) error {
		verified = true
		return nil
	}
	deps.Link = func(tempOutput, destination string) error {
		if !verified {
			t.Error("Link called before verification")
		}
		if destination != output {
			t.Errorf("Link destination = %q, want %q", destination, output)
		}
		assertFileContents(t, tempOutput, "rendered")
		if err := os.WriteFile(destination, []byte("racer"), 0o600); err != nil {
			t.Fatal(err)
		}
		return fmt.Errorf("link failed: %w", os.ErrExist)
	}

	_, err := Run(context.Background(), cli.Config{
		Input: "input.mp3", Model: "model.bin", Output: output, Language: "auto", Accent: "#112233",
	}, deps, &bytes.Buffer{})
	if Code(err) != 2 {
		t.Fatalf("Code(error) = %d, want 2 (%v)", Code(err), err)
	}
	var pipelineErr *Error
	if !errors.As(err, &pipelineErr) || pipelineErr.Op != "publishing output" {
		t.Fatalf("error = %v, want pipeline Error op %q", err, "publishing output")
	}
	assertFileContents(t, output, "racer")
	assertCleaned(t, workspaceParent, output)
}

func TestRunRenameFailurePreservesDestinationAndCleansTemporaryFiles(t *testing.T) {
	root := t.TempDir()
	workspaceParent := filepath.Join(root, "workspaces")
	if err := os.Mkdir(workspaceParent, 0o700); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "clip.mp4")
	if err := os.WriteFile(output, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := []string{}
	runner := &pipelineRunner{t: t, calls: &calls}
	deps := successfulDependencies(t, runner, workspaceParent, &calls)
	verified := false
	deps.Verify = func(context.Context, tools.Runner, string, string, time.Duration) error {
		verified = true
		return nil
	}
	deps.Rename = func(tempOutput, destination string) error {
		if !verified {
			t.Error("Rename called before verification")
		}
		if destination != output {
			t.Errorf("Rename destination = %q, want %q", destination, output)
		}
		assertFileContents(t, tempOutput, "rendered")
		assertFileContents(t, destination, "old")
		return errors.New("rename failed")
	}

	_, err := Run(context.Background(), cli.Config{
		Input: "input.mp3", Model: "model.bin", Output: output, Language: "auto", Accent: "#112233", Force: true,
	}, deps, &bytes.Buffer{})
	if Code(err) != 5 {
		t.Fatalf("Code(error) = %d, want 5 (%v)", Code(err), err)
	}
	var pipelineErr *Error
	if !errors.As(err, &pipelineErr) || pipelineErr.Op != "publishing output" {
		t.Fatalf("error = %v, want pipeline Error op %q", err, "publishing output")
	}
	assertFileContents(t, output, "old")
	assertCleaned(t, workspaceParent, output)
}

func TestCodeUsesStableExitCodes(t *testing.T) {
	if Code(nil) != 0 {
		t.Fatalf("Code(nil) = %d, want 0", Code(nil))
	}
	if Code(errors.New("unknown")) != 5 {
		t.Fatalf("Code(unknown) = %d, want 5", Code(errors.New("unknown")))
	}
	if Code(&Error{Code: 3, Op: "test", Err: errors.New("failure")}) != 3 {
		t.Fatal("Code did not preserve pipeline error code")
	}
	if Code(&Error{Code: 4, Op: "test", Err: context.Canceled}) != 130 {
		t.Fatal("cancellation did not take precedence")
	}
}

func successfulDependencies(t *testing.T, runner *pipelineRunner, workspaceParent string, calls *[]string) Dependencies {
	t.Helper()
	return Dependencies{
		Runner: runner,
		Preflight: func(context.Context, tools.Runner, string) (tools.Paths, error) {
			*calls = append(*calls, "preflight")
			return tools.Paths{FFmpeg: "ffmpeg", FFprobe: "ffprobe", Whisper: "whisper"}, nil
		},
		Probe: func(context.Context, tools.Runner, string, string) (media.InputInfo, error) {
			*calls = append(*calls, "probe")
			return media.InputInfo{Duration: 10 * time.Second, HasAudio: true, Title: "metadata title", CoverStream: 4}, nil
		},
		Transcribe: func(context.Context, tools.Runner, transcribe.Request) ([]transcribe.Word, error) {
			*calls = append(*calls, "transcribe")
			return []transcribe.Word{{Text: "hello", Start: 0, End: time.Second}}, nil
		},
		Render: func(_ context.Context, _ tools.Runner, request media.RenderRequest) error {
			*calls = append(*calls, "render")
			return os.WriteFile(request.Output, []byte("rendered"), 0o600)
		},
		Verify: func(context.Context, tools.Runner, string, string, time.Duration) error {
			*calls = append(*calls, "verify")
			return nil
		},
		CreateTemp: os.CreateTemp,
		Link:       os.Link,
		Rename:     os.Rename,
		TempParent: workspaceParent,
	}
}

func containsAdjacent(values []string, first, second string) bool {
	for i := 0; i+1 < len(values); i++ {
		if values[i] == first && values[i+1] == second {
			return true
		}
	}
	return false
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("%s contains %q, want %q", path, data, want)
	}
}

func assertCleaned(t *testing.T, workspaceParent, output string) {
	t.Helper()
	entries, err := os.ReadDir(workspaceParent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("workspace parent still contains %v", entries)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(output), "."+filepath.Base(output)+".volnorez-*.tmp.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Errorf("temporary outputs remain: %v", matches)
	}
}
