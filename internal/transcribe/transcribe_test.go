package transcribe

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"volnorez/internal/tools"
)

type transcribeRunner struct {
	command tools.Command
	err     error
}

func (r *transcribeRunner) LookPath(string) (string, error) { return "", errors.New("not implemented") }

func (r *transcribeRunner) Run(_ context.Context, command tools.Command) (tools.Result, error) {
	r.command = command
	return tools.Result{}, r.err
}

func TestParseFullJSONMergesStandalonePunctuation(t *testing.T) {
	b, err := os.ReadFile("testdata/words.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseFullJSON(b, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	want := []Word{
		{Text: "Привет,", Start: 0, End: 760 * time.Millisecond},
		{Text: "мир", Start: 760 * time.Millisecond, End: 1200 * time.Millisecond},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v; want %#v", got, want)
	}
}

func TestRunInvokesWhisperAndParsesOutput(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Join(dir, "timed")
	fixture, err := os.ReadFile("testdata/words.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prefix+".json", fixture, 0o600); err != nil {
		t.Fatal(err)
	}
	r := &transcribeRunner{}

	got, err := Run(context.Background(), r, Request{
		Whisper:      "WHISPER",
		Model:        "MODEL",
		WAV:          "WAV",
		Language:     "LANGUAGE",
		OutputPrefix: prefix,
		Duration:     2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantWords := []Word{{Text: "Привет,", End: 760 * time.Millisecond}, {Text: "мир", Start: 760 * time.Millisecond, End: 1200 * time.Millisecond}}
	if !reflect.DeepEqual(got, wantWords) {
		t.Fatalf("words = %#v, want %#v", got, wantWords)
	}
	wantCommand := tools.Command{Name: "WHISPER", Args: []string{"-m", "MODEL", "-f", "WAV", "-l", "LANGUAGE", "-ml", "1", "-ojf", "-of", prefix, "-np"}}
	if r.command.Name != wantCommand.Name || !reflect.DeepEqual(r.command.Args, wantCommand.Args) {
		t.Fatalf("command = %#v, want %#v", r.command, wantCommand)
	}
}

func TestParseFullJSONRejectsInvalidTranscript(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "empty transcription", data: `{"transcription":[]}`},
		{name: "missing offsets", data: `{"transcription":[{"offsets":{"from":0},"text":"word"}]}`},
		{name: "negative timestamp", data: `{"transcription":[{"offsets":{"from":-1,"to":10},"text":"word"}]}`},
		{name: "decreasing timestamps", data: `{"transcription":[{"offsets":{"from":10,"to":5},"text":"word"}]}`},
		{name: "punctuation only", data: `{"transcription":[{"offsets":{"from":0,"to":10},"text":" ,"}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseFullJSON([]byte(test.data), 2*time.Second)
			if err == nil {
				t.Fatal("ParseFullJSON returned nil error")
			}
		})
	}
}

func TestParseFullJSONClampsOnlyFinalEndToSelectedDuration(t *testing.T) {
	data := []byte(`{"transcription":[{"offsets":{"from":0,"to":1000},"text":"first"},{"offsets":{"from":1000,"to":2100},"text":"second"}]}`)
	got, err := ParseFullJSON(data, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	want := []Word{{Text: "first", Start: 0, End: time.Second}, {Text: "second", Start: time.Second, End: 2 * time.Second}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestRunWrapsWhisperFailure(t *testing.T) {
	r := &transcribeRunner{err: errors.New("exit status 1")}
	_, err := Run(context.Background(), r, Request{Whisper: "WHISPER"})
	if err == nil || !strings.Contains(err.Error(), "whisper") {
		t.Fatalf("error = %v", err)
	}
}
