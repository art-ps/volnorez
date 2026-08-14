package subtitles

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"volnorez/internal/transcribe"
)

func TestWriteASSHighlightsExactlyOneWord(t *testing.T) {
	phrases := Group([]transcribe.Word{
		{Text: "Привет", Start: 0, End: 400 * time.Millisecond},
		{Text: "мир", Start: 400 * time.Millisecond, End: 900 * time.Millisecond},
	})
	var out bytes.Buffer
	err := WriteASS(&out, Document{
		Duration: time.Second, Title: "Новый выпуск", FontFamily: "Noto Sans", Accent: "#FFD84D", Phrases: phrases,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if strings.Count(s, "Dialogue: 1,") != 2 {
		t.Fatalf("subtitle events:\n%s", s)
	}
	if !strings.Contains(s, `Привет{\c&HFFFFFF&} мир`) || !strings.Contains(s, `Привет {\c&H4DD8FF&}мир`) {
		t.Fatalf("missing active-word transitions:\n%s", s)
	}
}

func TestWriteASSEscapesDialogueText(t *testing.T) {
	phrases := []Phrase{{Words: []transcribe.Word{{Text: `a{b}\\c,d`, Start: 0, End: time.Second}}, Text: `a{b}\\c,d`}}
	var out bytes.Buffer
	if err := WriteASS(&out, Document{Duration: time.Second, FontFamily: "Noto Sans", Accent: "#010203", Phrases: phrases}); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, `a\{b\}\\\\c\,d`) {
		t.Fatalf("unescaped dialogue text:\n%s", s)
	}
}

func TestWriteASSEscapesNewlineInTitle(t *testing.T) {
	var out bytes.Buffer
	if err := WriteASS(&out, Document{Duration: time.Second, Title: "первая\nвторая", FontFamily: "Noto Sans", Accent: "#010203"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `первая\Nвторая`) {
		t.Fatalf("title newline not escaped:\n%s", out.String())
	}
}

func TestWriteASSConvertsAccentRGBToBGR(t *testing.T) {
	var out bytes.Buffer
	if err := WriteASS(&out, Document{Duration: time.Second, FontFamily: "Noto Sans", Accent: "#102030"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `&H302010&`) {
		t.Fatalf("accent not converted:\n%s", out.String())
	}
}

func TestWriteASSUsesDefaultAccentWhenEmpty(t *testing.T) {
	var out bytes.Buffer
	if err := WriteASS(&out, Document{Duration: time.Second, FontFamily: "Noto Sans"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `&H4DD8FF&`) {
		t.Fatalf("default accent not converted:\n%s", out.String())
	}
}

func TestWriteASSOmitsTitleEventWhenTitleEmpty(t *testing.T) {
	var out bytes.Buffer
	if err := WriteASS(&out, Document{Duration: time.Second, FontFamily: "Noto Sans", Accent: "#010203"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "Dialogue: 0,") {
		t.Fatalf("unexpected title event:\n%s", out.String())
	}
}

func TestWriteASSExtendsEventUntilNextWordStarts(t *testing.T) {
	phrases := Group([]transcribe.Word{
		{Text: "первый", Start: 0, End: 400 * time.Millisecond},
		{Text: "второй", Start: 900 * time.Millisecond, End: time.Second},
	})
	var out bytes.Buffer
	if err := WriteASS(&out, Document{Duration: 2 * time.Second, FontFamily: "Noto Sans", Accent: "#010203", Phrases: phrases}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Dialogue: 1,0:00:00.00,0:00:00.90") {
		t.Fatalf("first event not extended:\n%s", out.String())
	}
}

func TestWriteASSUsesPhraseLineBreak(t *testing.T) {
	phrases := []Phrase{{
		Words: timedWords("one two three", 400*time.Millisecond),
		Text:  `one\Ntwo three`,
	}}
	var out bytes.Buffer
	if err := WriteASS(&out, Document{Duration: 2 * time.Second, FontFamily: "Noto Sans", Accent: "#010203", Phrases: phrases}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `one{\c&HFFFFFF&}\Ntwo`) {
		t.Fatalf("phrase line break not preserved:\n%s", out.String())
	}
}
