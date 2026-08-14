package transcribe

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	"volnorez/internal/tools"
)

type Word struct {
	Text       string
	Start, End time.Duration
}

type Request struct {
	Whisper, Model, WAV, Language, OutputPrefix string
	Duration                                    time.Duration
}

type fullJSON struct {
	Transcription []segment `json:"transcription"`
}

type segment struct {
	Offsets struct {
		From *int64 `json:"from"`
		To   *int64 `json:"to"`
	} `json:"offsets"`
	Text string `json:"text"`
}

func Run(ctx context.Context, r tools.Runner, req Request) ([]Word, error) {
	_, err := r.Run(ctx, tools.Command{
		Name: req.Whisper,
		Args: []string{
			"-m", req.Model,
			"-f", req.WAV,
			"-l", req.Language,
			"-ml", "1",
			"-ojf",
			"-of", req.OutputPrefix,
			"-np",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("run whisper: %w", err)
	}
	data, err := os.ReadFile(req.OutputPrefix + ".json")
	if err != nil {
		return nil, fmt.Errorf("read whisper output: %w", err)
	}
	words, err := ParseFullJSON(data, req.Duration)
	if err != nil {
		return nil, fmt.Errorf("parse whisper output: %w", err)
	}
	return words, nil
}

func ParseFullJSON(data []byte, duration time.Duration) ([]Word, error) {
	if duration <= 0 {
		return nil, fmt.Errorf("duration must be positive")
	}
	var output fullJSON
	if err := json.Unmarshal(data, &output); err != nil {
		return nil, fmt.Errorf("decode full JSON: %w", err)
	}
	if len(output.Transcription) == 0 {
		return nil, fmt.Errorf("transcription is empty")
	}

	words := make([]Word, 0, len(output.Transcription))
	var previousEnd time.Duration
	for i, entry := range output.Transcription {
		if entry.Offsets.From == nil || entry.Offsets.To == nil {
			return nil, fmt.Errorf("transcription segment %d has missing offsets", i)
		}
		start := time.Duration(*entry.Offsets.From) * time.Millisecond
		end := time.Duration(*entry.Offsets.To) * time.Millisecond
		if start < 0 || end < 0 {
			return nil, fmt.Errorf("transcription segment %d has negative offsets", i)
		}
		if end < start || start < previousEnd {
			return nil, fmt.Errorf("transcription segment %d has decreasing offsets", i)
		}
		if i == len(output.Transcription)-1 {
			if end > duration {
				end = duration
			}
		} else if end > duration {
			return nil, fmt.Errorf("transcription segment %d exceeds duration", i)
		}
		if end < start {
			return nil, fmt.Errorf("transcription segment %d exceeds duration", i)
		}
		previousEnd = end

		text := strings.TrimSpace(entry.Text)
		if text == "" {
			continue
		}
		if punctuationOnly(text) {
			if len(words) == 0 {
				return nil, fmt.Errorf("transcription segment %d has punctuation without a preceding word", i)
			}
			words[len(words)-1].Text += text
			words[len(words)-1].End = end
			continue
		}
		words = append(words, Word{Text: text, Start: start, End: end})
	}
	if len(words) == 0 {
		return nil, fmt.Errorf("transcription has no words")
	}
	return words, nil
}

func punctuationOnly(text string) bool {
	for _, r := range text {
		if !unicode.IsPunct(r) {
			return false
		}
	}
	return true
}
