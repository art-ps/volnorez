package subtitles

import (
	"strings"
	"time"

	"volnorez/internal/transcribe"
)

const (
	maxWords      = 7
	maxRunes      = 42
	maxLineRunes  = 24
	maxPhraseSpan = 3500 * time.Millisecond
)

type Phrase struct {
	Words []transcribe.Word
	Text  string
}

func Group(words []transcribe.Word) []Phrase {
	var phrases []Phrase
	var current []transcribe.Word
	for _, word := range words {
		if len(current) > 0 && exceedsLimit(current, word) {
			phrases = append(phrases, newPhrase(current))
			current = nil
		}
		current = append(current, word)
		if terminalPunctuation(word.Text) {
			phrases = append(phrases, newPhrase(current))
			current = nil
		}
	}
	if len(current) > 0 {
		phrases = append(phrases, newPhrase(current))
	}
	return phrases
}

func exceedsLimit(current []transcribe.Word, word transcribe.Word) bool {
	if len(current) >= maxWords || word.End-current[0].Start > maxPhraseSpan {
		return true
	}
	return len([]rune(wordsText(current)))+1+len([]rune(word.Text)) > maxRunes
}

func newPhrase(words []transcribe.Word) Phrase {
	phraseWords := append([]transcribe.Word(nil), words...)
	return Phrase{Words: phraseWords, Text: balancedText(phraseWords)}
}

func wordsText(words []transcribe.Word) string {
	text := make([]string, len(words))
	for i, word := range words {
		text[i] = word.Text
	}
	return strings.Join(text, " ")
}

func balancedText(words []transcribe.Word) string {
	if len(words) == 2 && len([]rune(wordsText(words))) > maxLineRunes &&
		len([]rune(words[0].Text)) <= maxLineRunes && len([]rune(words[1].Text)) <= maxLineRunes {
		return words[0].Text + `\N` + words[1].Text
	}
	if len(words) < 3 {
		return wordsText(words)
	}

	bestSplit := 0
	bestDifference := -1
	for split := 1; split < len(words); split++ {
		left := len([]rune(wordsText(words[:split])))
		right := len([]rune(wordsText(words[split:])))
		difference := left - right
		if difference < 0 {
			difference = -difference
		}
		if bestDifference < 0 || difference < bestDifference {
			bestSplit, bestDifference = split, difference
		}
	}
	return wordsText(words[:bestSplit]) + `\N` + wordsText(words[bestSplit:])
}

func terminalPunctuation(text string) bool {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return false
	}
	switch runes[len(runes)-1] {
	case '.', '!', '?', '…':
		return true
	}
	return false
}
