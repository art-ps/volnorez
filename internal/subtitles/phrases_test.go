package subtitles

import (
	"strings"
	"testing"
	"time"

	"volnorez/internal/transcribe"
)

func TestGroupStopsAtTerminalPunctuation(t *testing.T) {
	words := timedWords("первое. второе третье", 400*time.Millisecond)
	got := Group(words)
	if len(got) != 2 || len(got[0].Words) != 1 || len(got[1].Words) != 2 {
		t.Fatalf("groups = %#v", got)
	}
}

func TestGroupIgnoresNonTerminalPunctuation(t *testing.T) {
	words := timedWords("версия2.0 готова", 400*time.Millisecond)
	got := Group(words)
	if len(got) != 1 {
		t.Fatalf("groups = %#v", got)
	}
}

func TestGroupStopsAtSevenWords(t *testing.T) {
	words := timedWords("один два три четыре пять шесть семь восемь", 400*time.Millisecond)
	got := Group(words)
	if len(got) != 2 || len(got[0].Words) != 7 || len(got[1].Words) != 1 {
		t.Fatalf("groups = %#v", got)
	}
	if strings.Count(got[0].Text, `\N`) > 1 {
		t.Fatalf("more than two lines: %q", got[0].Text)
	}
}

func TestGroupStopsBeforeFortyThirdRune(t *testing.T) {
	words := timedWords("абвгдеёжзийклмнопрсту абвгдеёжзийклмнопрсту", 400*time.Millisecond)
	got := Group(words)
	if len(got) != 2 || len(got[0].Words) != 1 || len(got[1].Words) != 1 {
		t.Fatalf("groups = %#v", got)
	}
}

func TestGroupCountsCyrillicRunes(t *testing.T) {
	words := timedWords("абвгдеёжзийклмнопрст абвгдеёжзийклмнопрсту", 400*time.Millisecond)
	got := Group(words)
	if len(got) != 1 || len(got[0].Words) != 2 {
		t.Fatalf("groups = %#v", got)
	}
}

func TestGroupStopsBeforeThreeAndHalfSeconds(t *testing.T) {
	words := []transcribe.Word{
		{Text: "один", Start: 0, End: 2 * time.Second},
		{Text: "два", Start: 2 * time.Second, End: 3500 * time.Millisecond},
		{Text: "три", Start: 3500 * time.Millisecond, End: 3600 * time.Millisecond},
	}
	got := Group(words)
	if len(got) != 2 || len(got[0].Words) != 2 || len(got[1].Words) != 1 {
		t.Fatalf("groups = %#v", got)
	}
}

func TestGroupBalancesTwoLines(t *testing.T) {
	words := timedWords("один два три четыре пять шесть", 400*time.Millisecond)
	got := Group(words)
	if len(got) != 1 || got[0].Text != `один два три\Nчетыре пять шесть` {
		t.Fatalf("text = %q", got[0].Text)
	}
}

func timedWords(text string, step time.Duration) []transcribe.Word {
	parts := strings.Fields(text)
	words := make([]transcribe.Word, len(parts))
	for i, part := range parts {
		words[i] = transcribe.Word{Text: part, Start: time.Duration(i) * step, End: time.Duration(i+1) * step}
	}
	return words
}
