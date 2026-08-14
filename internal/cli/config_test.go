package cli

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseUsesModelFlagBeforeEnvironment(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "episode.mp3")
	model := filepath.Join(dir, "model.bin")
	if err := os.WriteFile(input, []byte("mp3"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(model, []byte("model"), 0o600); err != nil {
		t.Fatal(err)
	}
	getenv := func(key string) string {
		if key == "WHISPER_MODEL" {
			return "/env/model.bin"
		}
		return ""
	}
	cfg, err := Parse([]string{input, "--model", model}, getenv)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != model {
		t.Fatalf("model = %q", cfg.Model)
	}
	if filepath.Base(cfg.Output) != "episode.mp4" {
		t.Fatalf("output = %q", cfg.Output)
	}
}

func TestParseRejectsExistingOutputWithoutForce(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "выпуск.mp3")
	output := filepath.Join(dir, "выпуск.mp4")
	model := filepath.Join(dir, "model.bin")
	if err := os.WriteFile(input, []byte("mp3"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(model, []byte("model"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Parse([]string{input, "--model", model}, os.Getenv)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseRejectsMissingInput(t *testing.T) {
	if _, err := Parse(nil, os.Getenv); err == nil {
		t.Fatal("Parse returned nil error")
	}
}

func TestParseRejectsMissingModel(t *testing.T) {
	input, _ := writeInputAndModel(t)
	if _, err := Parse([]string{input}, func(string) string { return "" }); err == nil {
		t.Fatal("Parse returned nil error")
	}
}

func TestParseRejectsNonMP3Input(t *testing.T) {
	_, model := writeInputAndModel(t)
	input := filepath.Join(t.TempDir(), "episode.wav")
	if err := os.WriteFile(input, []byte("wav"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse([]string{input, "--model", model}, os.Getenv); err == nil {
		t.Fatal("Parse returned nil error")
	}
}

func TestParseRejectsInvalidAccent(t *testing.T) {
	input, model := writeInputAndModel(t)
	if _, err := Parse([]string{input, "--model", model, "--accent", "orange"}, os.Getenv); err == nil {
		t.Fatal("Parse returned nil error")
	}
}

func TestParseRejectsNegativeStart(t *testing.T) {
	input, model := writeInputAndModel(t)
	if _, err := Parse([]string{input, "--model", model, "--start=-1"}, os.Getenv); err == nil {
		t.Fatal("Parse returned nil error")
	}
}

func TestParseRejectsZeroDuration(t *testing.T) {
	input, model := writeInputAndModel(t)
	if _, err := Parse([]string{input, "--model", model, "--duration", "0"}, os.Getenv); err == nil {
		t.Fatal("Parse returned nil error")
	}
}

func TestParseRejectsTrailingArguments(t *testing.T) {
	input, model := writeInputAndModel(t)
	if _, err := Parse([]string{input, "--model", model, "unexpected"}, os.Getenv); err == nil {
		t.Fatal("Parse returned nil error")
	}
}

func TestParseHelpDoesNotRequireInput(t *testing.T) {
	_, err := Parse([]string{"--help"}, os.Getenv)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("error = %v; want flag.ErrHelp", err)
	}
}

func TestUsageListsEveryFlagWithItsDefault(t *testing.T) {
	usage := Usage()
	if !strings.HasPrefix(usage, "Usage: volnorez INPUT [flags]\n") {
		t.Fatalf("usage = %q", usage)
	}
	for _, want := range []string{
		"--model (default \"\")", "--cover (default \"\")", "--output (default \"\")",
		"--language (default \"auto\")", "--start (default \"0\")", "--duration (default \"\")",
		"--accent (default \"#FFD84D\")", "--title (default \"\")", "--font (default \"\")",
		"--whisper-bin (default \"\")", "--force (default false)", "--verbose (default false)",
	} {
		if !strings.Contains(usage, want) {
			t.Fatalf("usage does not contain %q:\n%s", want, usage)
		}
	}
}

func TestParseAllowsExistingOutputWithForce(t *testing.T) {
	input, model := writeInputAndModel(t)
	output := strings.TrimSuffix(input, filepath.Ext(input)) + ".mp4"
	if err := os.WriteFile(output, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Parse([]string{input, "--model", model, "--force"}, os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Force {
		t.Fatal("Force = false")
	}
}

func TestParseRejectsNonMP4Output(t *testing.T) {
	input, model := writeInputAndModel(t)
	output := filepath.Join(t.TempDir(), "episode.mov")
	if _, err := Parse([]string{input, "--model", model, "--output", output}, os.Getenv); err == nil {
		t.Fatal("Parse returned nil error")
	}
}

func TestParseRejectsOutputEqualToInputEvenWithForce(t *testing.T) {
	input, model := writeInputAndModel(t)
	_, err := Parse([]string{input, "--model", model, "--output", input, "--force"}, os.Getenv)
	if err == nil {
		t.Fatal("Parse returned nil error")
	}
}

func writeInputAndModel(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, "episode.mp3")
	model := filepath.Join(dir, "model.bin")
	if err := os.WriteFile(input, []byte("mp3"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(model, []byte("model"), 0o600); err != nil {
		t.Fatal(err)
	}
	return input, model
}
