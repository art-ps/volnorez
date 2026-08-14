package media

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"volnorez/internal/tools"
)

type prepareRunner struct {
	command tools.Command
	err     error
}

func (r *prepareRunner) LookPath(string) (string, error) { return "", errors.New("not implemented") }

func (r *prepareRunner) Run(_ context.Context, command tools.Command) (tools.Result, error) {
	r.command = command
	return tools.Result{}, r.err
}

func TestExtractAudioBuildsPCMCommand(t *testing.T) {
	r := &prepareRunner{}
	err := ExtractAudio(context.Background(), r, "ffmpeg", "input.mp3", "workspace/speech.wav", Selection{
		Start:    1250 * time.Millisecond,
		Duration: 2750 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := tools.Command{Name: "ffmpeg", Args: []string{
		"-v", "error", "-y", "-ss", "1.250", "-t", "2.750", "-i", "input.mp3",
		"-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", "workspace/speech.wav",
	}}
	if !reflect.DeepEqual(r.command, want) {
		t.Fatalf("command = %#v, want %#v", r.command, want)
	}
}

func TestPrepareCoverPrefersExplicitCover(t *testing.T) {
	r := &prepareRunner{}
	err := PrepareCover(context.Background(), r, "ffmpeg", "input.mp3", "cover.jpg", "workspace/cover.png", 3)
	if err != nil {
		t.Fatal(err)
	}
	want := tools.Command{Name: "ffmpeg", Args: []string{
		"-v", "error", "-y", "-i", "cover.jpg", "-frames:v", "1", "workspace/cover.png",
	}}
	if !reflect.DeepEqual(r.command, want) {
		t.Fatalf("command = %#v, want %#v", r.command, want)
	}
}

func TestPrepareCoverExtractsEmbeddedCover(t *testing.T) {
	r := &prepareRunner{}
	err := PrepareCover(context.Background(), r, "ffmpeg", "input.mp3", "", "workspace/cover.png", 3)
	if err != nil {
		t.Fatal(err)
	}
	want := tools.Command{Name: "ffmpeg", Args: []string{
		"-v", "error", "-y", "-i", "input.mp3", "-map", "0:3", "-frames:v", "1", "workspace/cover.png",
	}}
	if !reflect.DeepEqual(r.command, want) {
		t.Fatalf("command = %#v, want %#v", r.command, want)
	}
}

func TestPrepareCoverRequiresExplicitOrEmbeddedCover(t *testing.T) {
	err := PrepareCover(context.Background(), &prepareRunner{}, "ffmpeg", "input.mp3", "", "workspace/cover.png", -1)
	if err == nil || !strings.Contains(err.Error(), "cover") {
		t.Fatalf("error = %v", err)
	}
}

func TestPreparationWrapsRunnerFailures(t *testing.T) {
	r := &prepareRunner{err: errors.New("exit status 1")}
	err := ExtractAudio(context.Background(), r, "ffmpeg", "input.mp3", "workspace/speech.wav", Selection{Duration: time.Second})
	if err == nil || !strings.Contains(err.Error(), "extract audio") {
		t.Fatalf("error = %v", err)
	}
}
