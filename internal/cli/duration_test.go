package cli

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := map[string]time.Duration{
		"75":        75 * time.Second,
		"01:15":     75 * time.Second,
		"1:02:03.5": time.Hour + 2*time.Minute + 3500*time.Millisecond,
	}
	for input, want := range tests {
		got, err := ParseDuration(input)
		if err != nil || got != want {
			t.Fatalf("ParseDuration(%q) = %v, %v; want %v", input, got, err, want)
		}
	}
}

func TestParseDurationRejectsNegativeValue(t *testing.T) {
	if _, err := ParseDuration("-1"); err == nil {
		t.Fatal("ParseDuration(-1) returned nil error")
	}
}

func TestParseDurationRejectsFractionalOverflow(t *testing.T) {
	if _, err := ParseDuration("2562047:47:16.854775808"); err == nil {
		t.Fatal("ParseDuration returned nil error")
	}
}
