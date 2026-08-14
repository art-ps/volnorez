package media

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"volnorez/internal/tools"
)

const probeJSON = `{
  "format":{"duration":"65.250","tags":{"title":"Новый выпуск"}},
  "streams":[
    {"index":0,"codec_type":"audio"},
    {"index":1,"codec_type":"video","disposition":{"attached_pic":1}}
  ]
}`

type probeRunner struct {
	result  tools.Result
	err     error
	command tools.Command
}

func (r *probeRunner) LookPath(string) (string, error) { return "", errors.New("not implemented") }

func (r *probeRunner) Run(_ context.Context, command tools.Command) (tools.Result, error) {
	r.command = command
	return r.result, r.err
}

func runnerReturning(stdout []byte) *probeRunner {
	return &probeRunner{result: tools.Result{Stdout: stdout}}
}

func TestProbeFindsAudioTitleAndCover(t *testing.T) {
	r := runnerReturning([]byte(probeJSON))
	got, err := Probe(context.Background(), r, "/bin/ffprobe", "/tmp/in.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if got.Duration != 65250*time.Millisecond || got.Title != "Новый выпуск" || got.CoverStream != 1 || !got.HasAudio {
		t.Fatalf("got %#v", got)
	}
	wantCommand := tools.Command{
		Name: "/bin/ffprobe",
		Args: []string{"-v", "error", "-show_entries", "format=duration:format_tags=title:stream=index,codec_type:stream_disposition=attached_pic", "-of", "json", "/tmp/in.mp3"},
	}
	if r.command.Name != wantCommand.Name || strings.Join(r.command.Args, "\x00") != strings.Join(wantCommand.Args, "\x00") {
		t.Fatalf("command = %#v, want %#v", r.command, wantCommand)
	}
}

func TestProbeRejectsInvalidJSONAndDuration(t *testing.T) {
	for _, test := range []struct {
		name   string
		output string
	}{
		{name: "invalid JSON", output: "not json"},
		{name: "invalid duration", output: `{"format":{"duration":"not-a-number"},"streams":[{"codec_type":"audio"}]}`},
		{name: "not a number", output: `{"format":{"duration":"NaN"},"streams":[{"codec_type":"audio"}]}`},
		{name: "zero duration", output: `{"format":{"duration":"0"},"streams":[{"codec_type":"audio"}]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Probe(context.Background(), runnerReturning([]byte(test.output)), "/bin/ffprobe", "/tmp/in.mp3")
			if err == nil {
				t.Fatal("Probe returned nil error")
			}
			if strings.Contains(err.Error(), "%!w") {
				t.Fatalf("error = %q", err)
			}
		})
	}
}

func TestProbeRejectsMissingAudio(t *testing.T) {
	output := `{"format":{"duration":"65.250"},"streams":[{"index":1,"codec_type":"video"}]}`
	_, err := Probe(context.Background(), runnerReturning([]byte(output)), "/bin/ffprobe", "/tmp/in.mp3")
	if err == nil || !strings.Contains(err.Error(), "audio") {
		t.Fatalf("error = %v", err)
	}
}

func TestProbeUsesMinusOneWithoutCover(t *testing.T) {
	output := `{"format":{"duration":"65.250"},"streams":[{"index":0,"codec_type":"audio"}]}`
	got, err := Probe(context.Background(), runnerReturning([]byte(output)), "/bin/ffprobe", "/tmp/in.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if got.CoverStream != -1 {
		t.Fatalf("CoverStream = %d, want -1", got.CoverStream)
	}
}

func TestSelectInterval(t *testing.T) {
	info := InputInfo{Duration: 10 * time.Second}
	tests := []struct {
		name        string
		start       time.Duration
		duration    time.Duration
		hasDuration bool
		want        Selection
		wantErr     bool
	}{
		{name: "whole file", want: Selection{Duration: 10 * time.Second}},
		{name: "explicit selection", start: 2 * time.Second, duration: 3 * time.Second, hasDuration: true, want: Selection{Start: 2 * time.Second, Duration: 3 * time.Second}},
		{name: "start at duration", start: 10 * time.Second, wantErr: true},
		{name: "negative start", start: -time.Second, wantErr: true},
		{name: "zero duration", duration: 0, hasDuration: true, wantErr: true},
		{name: "interval beyond source", start: 8 * time.Second, duration: 3 * time.Second, hasDuration: true, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := SelectInterval(info, test.start, test.duration, test.hasDuration)
			if test.wantErr {
				if err == nil {
					t.Fatal("SelectInterval returned nil error")
				}
				if !strings.Contains(err.Error(), test.start.String()) || !strings.Contains(err.Error(), info.Duration.String()) {
					t.Fatalf("error = %q; must include start %q and input duration %q", err, test.start, info.Duration)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("selection = %#v, want %#v", got, test.want)
			}
		})
	}
}
