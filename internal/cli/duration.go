package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const maxDurationNanos = uint64(1<<63 - 1)

var (
	secondsPattern = regexp.MustCompile(`^\d+$`)
	minutesPattern = regexp.MustCompile(`^\d{2}:\d{2}$`)
	hoursPattern   = regexp.MustCompile(`^\d+:\d{2}:\d{2}(?:\.\d+)?$`)
)

// ParseDuration accepts integer seconds, MM:SS, and HH:MM:SS[.mmm].
func ParseDuration(raw string) (time.Duration, error) {
	var hours, minutes, seconds uint64
	var fraction string

	switch {
	case secondsPattern.MatchString(raw):
		value, err := parseDurationNumber(raw)
		if err != nil {
			return 0, invalidDuration(raw)
		}
		seconds = value
	case minutesPattern.MatchString(raw):
		parts := strings.Split(raw, ":")
		var err error
		minutes, err = parseDurationNumber(parts[0])
		if err != nil {
			return 0, invalidDuration(raw)
		}
		seconds, err = parseDurationNumber(parts[1])
		if err != nil || seconds >= 60 {
			return 0, invalidDuration(raw)
		}
	case hoursPattern.MatchString(raw):
		parts := strings.Split(raw, ":")
		var err error
		hours, err = parseDurationNumber(parts[0])
		if err != nil {
			return 0, invalidDuration(raw)
		}
		minutes, err = parseDurationNumber(parts[1])
		if err != nil || minutes >= 60 {
			return 0, invalidDuration(raw)
		}
		var secondsText string
		secondsText, fraction, _ = strings.Cut(parts[2], ".")
		seconds, err = parseDurationNumber(secondsText)
		if err != nil || seconds >= 60 {
			return 0, invalidDuration(raw)
		}
	default:
		return 0, invalidDuration(raw)
	}

	total := uint64(0)
	var err error
	if total, err = addDurationPart(total, hours, uint64(time.Hour)); err != nil {
		return 0, fmt.Errorf("duration %q is too large", raw)
	}
	if total, err = addDurationPart(total, minutes, uint64(time.Minute)); err != nil {
		return 0, fmt.Errorf("duration %q is too large", raw)
	}
	if total, err = addDurationPart(total, seconds, uint64(time.Second)); err != nil {
		return 0, fmt.Errorf("duration %q is too large", raw)
	}
	if fraction != "" {
		if len(fraction) > 9 {
			return 0, invalidDuration(raw)
		}
		nanoseconds, err := parseDurationNumber(fraction)
		if err != nil {
			return 0, invalidDuration(raw)
		}
		for range 9 - len(fraction) {
			nanoseconds *= 10
		}
		if total > maxDurationNanos-nanoseconds {
			return 0, fmt.Errorf("duration %q is too large", raw)
		}
		total += nanoseconds
	}
	return time.Duration(total), nil
}

func addDurationPart(total, value, unit uint64) (uint64, error) {
	if value != 0 && value > (maxDurationNanos-total)/unit {
		return 0, fmt.Errorf("duration overflow")
	}
	return total + value*unit, nil
}

func parseDurationNumber(raw string) (uint64, error) {
	return strconv.ParseUint(raw, 10, 64)
}

func invalidDuration(raw string) error {
	return fmt.Errorf("invalid duration %q", raw)
}
