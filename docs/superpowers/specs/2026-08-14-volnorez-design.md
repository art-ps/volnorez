# Volnorez CLI Design

**Date:** 2026-08-14

**Status:** Approved

## Product

`volnorez` turns one MP3 file into one vertical MP4 suitable for stories and podcast announcements. It runs locally, uses an existing `whisper.cpp` model for transcription, and delegates media processing to FFmpeg.

The MVP targets macOS and Linux. Its defining experience is one command, sensible visual defaults, no editor project, and no cloud service.

## Goals

- Convert either the whole MP3 or a selected interval into a vertical video.
- Render a small circular cover, a large waveform reacting to the audio, and synchronized subtitles.
- Keep each subtitle phrase stable while highlighting the currently spoken word.
- Use an installed `whisper-cli` and a model selected by CLI flag or environment variable.
- Produce a single playable MP4 and never leave a partial result at the output path.
- Support paths containing spaces and Cyrillic characters on macOS and Linux.

## Non-goals

- Windows support.
- Cloud transcription.
- A GUI or interactive editor.
- Multiple layout presets, template files, or arbitrary element positioning.
- Automatic background generation.
- Speaker diarization, translation, transcript editing, or batch processing.
- Native audio decoding, waveform rendering, speech recognition, or video encoding in Go.

## User interface

Minimal invocation:

```bash
volnorez episode.mp3 --model /Users/me/models/ggml-medium.bin
```

The default output is `episode.mp4` next to the source file.

Full MVP invocation:

```bash
volnorez episode.mp3 \
  --model /Users/me/models/ggml-medium.bin \
  --cover cover.jpg \
  --output promo.mp4 \
  --language auto \
  --start 01:20 \
  --duration 00:45 \
  --accent "#FFD84D" \
  --title "Новый выпуск" \
  --font /Users/me/fonts/Inter-SemiBold.ttf \
  --whisper-bin /opt/homebrew/bin/whisper-cli \
  --verbose
```

### Arguments and flags

- Positional `INPUT`: required path to an MP3 file.
- `--model PATH`: Whisper GGML model. Resolution order is this flag, then `WHISPER_MODEL`; absence is an error.
- `--cover PATH`: JPEG or PNG cover. Resolution order is this flag, then the MP3 embedded picture; absence is an error.
- `--output PATH`: destination MP4. Default is the input path with its extension changed to `.mp4`.
- `--language CODE`: Whisper language code or `auto`. Default is `auto`.
- `--start TIME`: fragment start. Accepts integer seconds, `MM:SS`, or `HH:MM:SS[.mmm]`. Default is zero.
- `--duration TIME`: fragment duration in the same formats. Default is the remainder of the input.
- `--accent COLOR`: six-digit RGB value in `#RRGGBB` form. Default is `#FFD84D`.
- `--title TEXT`: title above the cover. Resolution order is this flag, the MP3 title tag, then no title.
- `--font PATH`: TTF or OTF subtitle font. The binary embeds an open-licensed Noto Sans regular/bold default so the default result does not depend on system fonts.
- `--whisper-bin PATH`: path to `whisper-cli`. Default is lookup in `PATH`.
- `--force`: allow replacing an existing output file. Without it, an existing output is an error.
- `--verbose`: print child-process commands and their complete diagnostic output.

The program prints only three normal progress stages to stderr: `checking input`, `transcribing`, and `rendering`. On success it prints the final path to stdout. This keeps stdout script-friendly.

## Output contract

- Container: MP4 with `faststart` metadata.
- Canvas: 1080×1920 pixels.
- Frame rate: constant 30 FPS.
- Video: H.264, `yuv420p` for broad mobile compatibility.
- Audio: AAC stereo, 192 kbit/s, derived from the selected source interval.
- Duration: selected interval within 0.2 seconds.
- Layout: dark solid background, optional title, small circular cover in the upper third, large centered waveform, and subtitles in the lower safe area.
- Accent color: waveform and current subtitle word.

## Architecture

The Go program is an orchestrator. It invokes external tools with argument arrays, never through a shell, so spaces and non-ASCII paths need no shell escaping.

### Packages

- `cmd/volnorez`: entry point, signal-aware context, exit-code mapping.
- `internal/cli`: argument parsing, defaults, validation, and help text.
- `internal/tools`: executable discovery, capability checks, and context-aware child-process execution.
- `internal/media`: `ffprobe` inspection, fragment extraction, embedded-cover extraction, render invocation, and output verification.
- `internal/transcribe`: `whisper-cli` invocation and parsing of full JSON output into normalized word timings.
- `internal/subtitles`: punctuation normalization, phrase grouping, line balancing, ASS escaping, and per-word highlight events.
- `internal/pipeline`: orchestration, temporary workspace lifecycle, progress reporting, and atomic output publication.
- `internal/assets`: embedded default font files and their licenses.

Each package exposes typed Go data and command arguments. No package passes shell command strings to another package.

## Processing flow

1. Parse flags and resolve the input, output, model, optional cover, and optional font to absolute paths.
2. Reject an existing output unless `--force` is present. `--force` still writes a sibling temporary file first; it does not render directly over the old output.
3. Locate `ffmpeg`, `ffprobe`, and `whisper-cli`. Check `whisper-cli --help` for `--output-json-full` and `--max-len`; check the FFmpeg filter list for `showwaves`, `subtitles`, and `geq`, and the encoder list for `libx264` and `aac`. Fail before expensive work if a required capability is missing.
4. Read duration, audio-stream presence, title metadata, and attached-picture metadata with `ffprobe`. Validate that `start < input duration` and that the selected interval is positive and contained in the input.
5. Create a private temporary directory. Extract the selected interval as mono PCM WAV at 16 kHz. Resolve the cover by copying and normalizing `--cover` or extracting the attached MP3 picture, then convert it to PNG. If `--font` is absent, extract the embedded default fonts into this directory for FFmpeg/libass.
6. Run `whisper-cli` on the normalized WAV with the selected model, `--language`, `--max-len 1`, `--output-json-full`, and an output prefix inside the temporary directory. The current official CLI documents MP3/WAV inputs, full JSON output, automatic language selection, and `--max-len`; word-level timing through `--max-len 1` is experimental, so the startup capability check and parser diagnostics are part of the compatibility boundary.
7. Parse non-empty timestamped transcription segments as words. Trim surrounding whitespace, attach standalone punctuation to the preceding word, reject negative or decreasing timestamps, and clamp the final word to the selected audio duration. An empty transcript is a rendering error because the product promise includes subtitles.
8. Group words into phrases. A phrase ends at sentence punctuation, seven words, 42 Unicode code points, or 3.5 seconds, whichever comes first. Balance each phrase over at most two lines at a word boundary; when possible, each line is at most 24 Unicode code points.
9. Generate an ASS file at 1080×1920. For every word interval, emit a dialogue event containing its whole phrase and color only the active word. Extend each event through the start of the following word so the phrase does not flicker during short gaps. Escape ASS control characters and explicit line breaks.
10. Render with one FFmpeg invocation. The normalized cover is scaled and circularly masked. `showwaves` reads the selected audio and generates the animated waveform. FFmpeg composes the title, cover, waveform, and ASS subtitles over the fixed dark background, reads the selected interval from the original MP3 for the AAC track, and writes a sibling temporary MP4.
11. Verify the temporary MP4 with `ffprobe`: it must contain one video stream and one audio stream, report 1080×1920, and match the selected duration within 0.2 seconds.
12. Publish from the sibling temporary file. Without `--force`, create the final name with `link`, which atomically fails if that name already exists, then unlink the temporary name. With `--force`, use a same-directory rename, which atomically replaces the old destination on macOS and Linux. Clean the private temporary directory on success, failure, or cancellation.

## Error behavior

Errors are one-line summaries by default and include a corrective action where possible. `--verbose` adds the underlying tool invocation and captured diagnostics.

Exit codes:

- `0`: success.
- `2`: invalid arguments or input metadata.
- `3`: missing executable, model, font, cover, or required external-tool capability.
- `4`: transcription failed or produced no usable timed words.
- `5`: media preparation, rendering, or final verification failed.
- `130`: interrupted by the user.

`SIGINT` and `SIGTERM` cancel the root context, terminate the active child process, remove temporary artifacts, and leave any pre-existing destination untouched.

## Testing

### Unit tests

- Parse accepted time forms and reject negative, malformed, overflowing, or out-of-range values.
- Apply flag, environment, metadata, and default precedence.
- Parse representative current `whisper.cpp` full JSON, including punctuation tokens and Cyrillic text.
- Reject missing, negative, decreasing, and empty word timings.
- Split phrases at every stated boundary and balance them over no more than two lines.
- Escape commas, braces, backslashes, newlines, and non-ASCII text in ASS.
- Generate adjacent subtitle events with exactly one active word per event.
- Build external-process argument arrays without shell quoting.

### Integration tests

Temporary fake executables capture arguments and simulate success, malformed output, non-zero exit, and cancellation. Tests cover executable discovery, capability checks, cleanup, output collision, `--force`, and exit-code mapping without requiring a Whisper model.

Saved full-JSON transcription fixtures drive a real FFmpeg render test. The test asserts streams, dimensions, frame rate, codecs, duration, and a non-static waveform region. It runs on macOS and Linux when FFmpeg is available.

### Optional end-to-end test

When both `WHISPER_MODEL` and `VOLNOREZ_E2E_MP3` are set, the selected local speech fixture runs through the real `whisper-cli` and FFmpeg pipeline. CI does not download a model or speech recording automatically.

## Acceptance criteria

- A minimal invocation with a valid MP3 and model creates exactly one final MP4.
- Whole-file and `--start`/`--duration` modes satisfy the output contract.
- Explicit cover overrides embedded art; absence of both fails before transcription.
- `--language auto` and an explicit language code are passed correctly to `whisper-cli`.
- The chosen model comes from `--model`, falling back to `WHISPER_MODEL` only when the flag is absent.
- The waveform visibly changes with audio amplitude.
- Subtitle phrases remain stable while the current word changes to the accent color.
- Existing output is preserved unless `--force` is supplied.
- Failure or cancellation leaves no partial final output.
- Paths containing spaces and Cyrillic text work on macOS and Linux.
- `go test ./...` passes without a Whisper model.

## Distribution

The release produces standalone Go binaries for macOS and Linux on amd64 and arm64. The binary embeds the default fonts and license text, but not FFmpeg, `whisper-cli`, or a Whisper model. Installation documentation lists those three runtime requirements and provides a preflight example using `volnorez --help` and a minimal conversion command.

## References

- [`whisper.cpp` CLI README](https://github.com/ggml-org/whisper.cpp/blob/master/examples/cli/README.md)
- [`whisper.cpp` CLI implementation](https://github.com/ggml-org/whisper.cpp/blob/master/examples/cli/cli.cpp)
