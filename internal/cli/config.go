package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const usageHeader = "Usage: volnorez INPUT [flags]"

var accentPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

type Config struct {
	Input, Model, Cover, Output, Language string
	Accent, Title, Font, WhisperBin       string
	Start, Duration                       time.Duration
	HasDuration, Force, Verbose           bool
}

type Error struct {
	ExitCode int
	Err      error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

func Code(err error) int {
	var classified *Error
	if errors.As(err, &classified) {
		return classified.ExitCode
	}
	return 2
}

type options struct {
	model, cover, output, language  string
	accent, title, font, whisperBin string
	start, duration                 string
	force, verbose                  bool
}

func newFlagSet(values *options, output io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet("volnorez", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&values.model, "model", "", "Whisper GGML model path")
	flags.StringVar(&values.cover, "cover", "", "JPEG or PNG cover path")
	flags.StringVar(&values.output, "output", "", "destination MP4 path")
	flags.StringVar(&values.language, "language", "auto", "Whisper language code or auto")
	flags.StringVar(&values.start, "start", "0", "fragment start time")
	flags.StringVar(&values.duration, "duration", "", "fragment duration")
	flags.StringVar(&values.accent, "accent", "#FFD84D", "six-digit RGB accent color")
	flags.StringVar(&values.title, "title", "", "title above the cover")
	flags.StringVar(&values.font, "font", "", "TTF or OTF subtitle font path")
	flags.StringVar(&values.whisperBin, "whisper-bin", "", "whisper-cli path")
	flags.BoolVar(&values.force, "force", false, "replace an existing output")
	flags.BoolVar(&values.verbose, "verbose", false, "print child-process diagnostics")
	flags.Usage = func() {
		fmt.Fprintln(output, usageHeader)
		flags.VisitAll(func(current *flag.Flag) {
			defaultValue := fmt.Sprintf("%q", current.DefValue)
			if _, ok := current.Value.(interface{ IsBoolFlag() bool }); ok {
				defaultValue = current.DefValue
			}
			fmt.Fprintf(output, "  --%s (default %s): %s\n", current.Name, defaultValue, current.Usage)
		})
	}
	return flags
}

func Usage() string {
	var output strings.Builder
	values := options{}
	newFlagSet(&values, &output).Usage()
	return output.String()
}

func Parse(args []string, getenv func(string) string) (Config, error) {
	values := options{}
	flags := newFlagSet(&values, io.Discard)
	if len(args) == 0 {
		return Config{}, fmt.Errorf("input MP3 is required")
	}
	if args[0] == "-h" || args[0] == "--help" {
		return Config{}, flags.Parse(args)
	}

	input, err := absolutePath(args[0])
	if err != nil {
		return Config{}, err
	}
	if !strings.EqualFold(filepath.Ext(input), ".mp3") {
		return Config{}, fmt.Errorf("input must be an MP3 file")
	}
	if err := requireExistingPath("input", input); err != nil {
		return Config{}, err
	}
	if err := flags.Parse(args[1:]); err != nil {
		return Config{}, err
	}
	if flags.NArg() != 0 {
		return Config{}, fmt.Errorf("unexpected trailing argument %q", flags.Arg(0))
	}

	if values.model == "" {
		values.model = getenv("WHISPER_MODEL")
	}
	if values.model == "" {
		return Config{}, resourceError(fmt.Errorf("Whisper model is required; use --model or WHISPER_MODEL"))
	}
	model, err := absolutePath(values.model)
	if err != nil {
		return Config{}, err
	}
	if err := requireExistingPath("model", model); err != nil {
		return Config{}, resourceError(err)
	}

	output := values.output
	if output == "" {
		output = strings.TrimSuffix(input, filepath.Ext(input)) + ".mp4"
	}
	output, err = absolutePath(output)
	if err != nil {
		return Config{}, err
	}
	if output == input {
		return Config{}, fmt.Errorf("output must not be the input file")
	}
	if !strings.EqualFold(filepath.Ext(output), ".mp4") {
		return Config{}, fmt.Errorf("output must be an MP4 file")
	}
	if !values.force {
		if _, err := os.Stat(output); err == nil {
			return Config{}, fmt.Errorf("output %q already exists; use --force to replace it", output)
		} else if !errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("check output %q: %w", output, err)
		}
	}
	if !accentPattern.MatchString(values.accent) {
		return Config{}, fmt.Errorf("accent must be in #RRGGBB form")
	}

	start, err := ParseDuration(values.start)
	if err != nil {
		return Config{}, fmt.Errorf("invalid --start: %w", err)
	}
	if start < 0 {
		return Config{}, fmt.Errorf("--start cannot be negative")
	}

	cfg := Config{
		Input: input, Model: model, Output: output, Language: values.language,
		Accent: values.accent, Title: values.title, Start: start,
		Force: values.force, Verbose: values.verbose,
	}
	if values.duration != "" {
		duration, err := ParseDuration(values.duration)
		if err != nil {
			return Config{}, fmt.Errorf("invalid --duration: %w", err)
		}
		if duration <= 0 {
			return Config{}, fmt.Errorf("--duration must be greater than zero")
		}
		cfg.Duration = duration
		cfg.HasDuration = true
	}
	if values.cover != "" {
		cfg.Cover, err = absolutePath(values.cover)
		if err != nil {
			return Config{}, err
		}
		if !hasExtension(cfg.Cover, ".jpg", ".jpeg", ".png") {
			return Config{}, fmt.Errorf("cover must be a JPEG or PNG file")
		}
		if err := requireExistingPath("cover", cfg.Cover); err != nil {
			return Config{}, resourceError(err)
		}
	}
	if values.font != "" {
		cfg.Font, err = absolutePath(values.font)
		if err != nil {
			return Config{}, err
		}
		if !hasExtension(cfg.Font, ".ttf", ".otf") {
			return Config{}, fmt.Errorf("font must be a TTF or OTF file")
		}
		if err := requireExistingPath("font", cfg.Font); err != nil {
			return Config{}, resourceError(err)
		}
	}
	if values.whisperBin != "" {
		cfg.WhisperBin, err = absolutePath(values.whisperBin)
		if err != nil {
			return Config{}, err
		}
		if err := requireExistingPath("whisper binary", cfg.WhisperBin); err != nil {
			return Config{}, resourceError(err)
		}
	}
	return cfg, nil
}

func resourceError(err error) error {
	return &Error{ExitCode: 3, Err: err}
}

func hasExtension(path string, allowed ...string) bool {
	extension := filepath.Ext(path)
	for _, candidate := range allowed {
		if strings.EqualFold(extension, candidate) {
			return true
		}
	}
	return false
}

func absolutePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", path, err)
	}
	return abs, nil
}

func requireExistingPath(label, path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s %q does not exist", label, path)
		}
		return fmt.Errorf("check %s %q: %w", label, path, err)
	}
	return nil
}
