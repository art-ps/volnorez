package tests

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"volnorez/internal/cli"
	"volnorez/internal/pipeline"
	"volnorez/internal/tools"
)

func TestPipelineWithWhisper(t *testing.T) {
	model := os.Getenv("WHISPER_MODEL")
	input := os.Getenv("VOLNOREZ_E2E_MP3")
	if model == "" || input == "" {
		t.Skip("set WHISPER_MODEL and VOLNOREZ_E2E_MP3")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cover := filepath.Join(dir, "cover.png")
	output := filepath.Join(dir, "e2e.mp4")
	runExternal(t, ffmpeg, "-v", "error", "-y", "-f", "lavfi", "-i",
		"color=c=0x8844cc:s=600x600:d=1", "-frames:v", "1", cover)
	absInput, err := filepath.Abs(input)
	if err != nil {
		t.Fatal(err)
	}
	absModel, err := filepath.Abs(model)
	if err != nil {
		t.Fatal(err)
	}
	cfg := cli.Config{
		Input: absInput, Model: absModel, Cover: cover, Output: output,
		Language: "auto", Accent: "#FFD84D", Duration: 5 * time.Second, HasDuration: true,
	}
	runner := tools.ExecRunner{Diagnostic: io.Discard}
	got, err := pipeline.Run(context.Background(), cfg, pipeline.DefaultDependencies(runner), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if got != output {
		t.Fatalf("output = %q", got)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatal(err)
	}
}
