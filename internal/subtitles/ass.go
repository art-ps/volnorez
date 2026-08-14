package subtitles

import (
	"fmt"
	"io"
	"strings"
	"time"

	"volnorez/internal/transcribe"
)

type Document struct {
	Duration   time.Duration
	Title      string
	FontFamily string
	Accent     string
	Phrases    []Phrase
}

func WriteASS(w io.Writer, document Document) error {
	accent, err := assColor(document.Accent)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, assHeader(document.FontFamily, accent)); err != nil {
		return err
	}
	if document.Title != "" {
		if err := writeEvent(w, 0, 0, document.Duration, "Title", escapeASS(document.Title)); err != nil {
			return err
		}
	}

	for phraseIndex, phrase := range document.Phrases {
		for wordIndex, word := range phrase.Words {
			end := word.End
			if next, ok := nextWord(document.Phrases, phraseIndex, wordIndex); ok {
				end = next.Start
			}
			if document.Duration > 0 && end > document.Duration {
				end = document.Duration
			}
			if err := writeEvent(w, 1, word.Start, end, "Subtitle", highlightText(phrase, wordIndex, accent)); err != nil {
				return err
			}
		}
	}
	return nil
}

func assHeader(fontFamily, accent string) string {
	return fmt.Sprintf(`[Script Info]
ScriptType: v4.00+
PlayResX: 1080
PlayResY: 1920

[V4+ Styles]
Format: Name,Fontname,Fontsize,PrimaryColour,SecondaryColour,OutlineColour,BackColour,Bold,Italic,Underline,StrikeOut,ScaleX,ScaleY,Spacing,Angle,BorderStyle,Outline,Shadow,Alignment,MarginL,MarginR,MarginV,Encoding
Style: Title,%s,56,&H00FFFFFF,&H00FFFFFF,&H00000000,&H00000000,-1,0,0,0,100,100,0,0,1,0,0,8,60,60,120,1
Style: Subtitle,%s,64,&H00FFFFFF,%s,&H00000000,&H00000000,-1,0,0,0,100,100,0,0,1,5,0,2,80,80,210,1

[Events]
Format: Layer,Start,End,Style,Name,MarginL,MarginR,MarginV,Effect,Text
`, escapeASS(fontFamily), escapeASS(fontFamily), accent)
}

func writeEvent(w io.Writer, layer int, start, end time.Duration, style, text string) error {
	_, err := fmt.Fprintf(w, "Dialogue: %d,%s,%s,%s,,0,0,0,,%s\n", layer, assTime(start), assTime(end), style, text)
	return err
}

func nextWord(phrases []Phrase, phraseIndex, wordIndex int) (word transcribe.Word, ok bool) {
	if wordIndex+1 < len(phrases[phraseIndex].Words) {
		return phrases[phraseIndex].Words[wordIndex+1], true
	}
	if phraseIndex+1 < len(phrases) && len(phrases[phraseIndex+1].Words) > 0 {
		return phrases[phraseIndex+1].Words[0], true
	}
	return transcribe.Word{}, false
}

func highlightText(phrase Phrase, active int, accent string) string {
	var text strings.Builder
	for i, word := range phrase.Words {
		if i > 0 {
			if lineBreakBefore(phrase, i) {
				text.WriteString(`\N`)
			} else {
				text.WriteByte(' ')
			}
		}
		if i == active {
			text.WriteString(`{\c`)
			text.WriteString(accent)
			text.WriteString(`}`)
		}
		text.WriteString(escapeASS(word.Text))
		if i == active && i+1 < len(phrase.Words) {
			text.WriteString(`{\c&HFFFFFF&}`)
		}
	}
	return text.String()
}

func lineBreakBefore(phrase Phrase, index int) bool {
	return phrase.Text == wordsText(phrase.Words[:index])+`\N`+wordsText(phrase.Words[index:])
}

func assColor(rgb string) (string, error) {
	if len(rgb) != 7 || rgb[0] != '#' {
		return "", fmt.Errorf("accent must be #RRGGBB")
	}
	for _, r := range rgb[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return "", fmt.Errorf("accent must be #RRGGBB")
		}
	}
	return "&H" + rgb[5:7] + rgb[3:5] + rgb[1:3] + "&", nil
}

func assTime(duration time.Duration) string {
	centiseconds := duration / (10 * time.Millisecond)
	hours := centiseconds / 360000
	minutes := (centiseconds / 6000) % 60
	seconds := (centiseconds / 100) % 60
	return fmt.Sprintf("%d:%02d:%02d.%02d", hours, minutes, seconds, centiseconds%100)
}

func escapeASS(text string) string {
	text = strings.ReplaceAll(text, "\\", `\\`)
	text = strings.ReplaceAll(text, "{", `\{`)
	text = strings.ReplaceAll(text, "}", `\}`)
	text = strings.ReplaceAll(text, ",", `\,`)
	return strings.ReplaceAll(text, "\n", `\N`)
}
