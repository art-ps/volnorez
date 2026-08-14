# Volnorez

`volnorez` turns one MP3 into a vertical 1080×1920 MP4: circular cover,
animated waveform, and word-highlighted subtitles. It runs locally with
FFmpeg and [whisper.cpp](https://github.com/ggml-org/whisper.cpp).

## Product example

```console
$ volnorez episode.mp3 --model ~/models/ggml-medium.bin --cover cover.jpg
checking input
transcribing
rendering
/absolute/path/to/episode.mp4
```

The three progress lines go to stderr. On success, stdout contains only the
absolute final MP4 path.

## Requirements

- `ffmpeg` and `ffprobe` on `PATH`, with filters `showwaves`, `subtitles`, and
  `geq`, plus `libx264` and `aac` encoders.
- `whisper-cli` from whisper.cpp on `PATH`, or supplied with `--whisper-bin`.
  It must support `--output-json-full`, `--max-len`, and `--language`.
- A local whisper.cpp GGML model, supplied with `--model` or `WHISPER_MODEL`.
- An MP3 with embedded cover art, or a JPEG/PNG supplied with `--cover`.

Each conversion performs preflight checks before reading or rendering media.
For example, a missing FFmpeg subtitle filter is reported as:

```text
volnorez: checking tools: check ffmpeg filters: missing capability subtitles
```

### macOS FFmpeg note

The default Homebrew `ffmpeg` at `/opt/homebrew/bin/ffmpeg` (8.1.2) may not
include the `subtitles` filter/libass. Install the full formula and put it
ahead of the default formula on `PATH`:

```bash
brew tap homebrew-ffmpeg/ffmpeg
brew install ffmpeg-full
export PATH="/opt/homebrew/opt/ffmpeg-full/bin:$PATH"
hash -r
```

Then rerun the command; preflight names any capability still missing. The
tested full build was `ffmpeg-full` 9.0.1 at
`/opt/homebrew/opt/ffmpeg-full/bin`.

## Install from a release

Download the binary matching your platform from the repository's Releases
page, verify it against `SHA256SUMS`, then install it. The release names are
`volnorez-darwin-amd64`, `volnorez-darwin-arm64`, `volnorez-linux-amd64`, and
`volnorez-linux-arm64`.

```bash
chmod +x volnorez-darwin-arm64
install -m 755 volnorez-darwin-arm64 /usr/local/bin/volnorez
```

Release archives include the binary, `README.md`, `THIRD_PARTY_NOTICES.md`,
and `SHA256SUMS`. They do not include FFmpeg, whisper.cpp, or a model.

## Build from source

Go 1.26.5 or newer is required by `go.mod`.

```bash
go build -trimpath -o volnorez ./cmd/volnorez
```

## Minimal command

```bash
volnorez episode.mp3 --model ~/models/ggml-medium.bin
```

The default output is `episode.mp4` next to the input. This command requires
embedded cover art; add `--cover cover.jpg` when the MP3 has none.

## Full flag reference

```text
volnorez INPUT [flags]
```

| Argument or flag | Default | Meaning |
| --- | --- | --- |
| `INPUT` | required | Source MP3 file. |
| `--model PATH` | `WHISPER_MODEL` | whisper.cpp GGML model path. |
| `--cover PATH` | embedded cover | JPEG or PNG cover path. |
| `--output PATH` | input with `.mp4` | Destination MP4 path. |
| `--language CODE` | `auto` | Whisper language code or `auto`. |
| `--start TIME` | `0` | Start time: seconds, `MM:SS`, or `HH:MM:SS[.mmm]`. |
| `--duration TIME` | remainder | Duration in the same formats; must be positive. |
| `--accent #RRGGBB` | `#FFD84D` | Waveform and active-word color. |
| `--title TEXT` | MP3 title tag | Title above the cover. |
| `--font PATH` | bundled Noto Sans | TTF or OTF subtitle font. |
| `--whisper-bin PATH` | `PATH` lookup | `whisper-cli` executable. |
| `--force` | off | Atomically replace an existing output. |
| `--verbose` | off | Stream child-process diagnostics to stderr. |

Run `volnorez --help` for the executable's current help text.

## Cover, title, and model precedence

- Model: `--model`, then `WHISPER_MODEL`; neither is an error.
- Cover: `--cover`, then embedded MP3 art; neither is an error.
- Title: `--title`, then the MP3 title tag; neither leaves the title absent.

## Output contract

The result is an MP4 with faststart metadata, H.264 `yuv420p` video at
1080×1920 and 30 FPS, and stereo AAC audio at 192 kbit/s. It uses the selected
input interval (within 0.2 seconds), a dark layout, circular cover, animated
waveform, and synchronized word highlighting. Rendering writes a sibling
temporary file and publishes it only after FFprobe verification, preserving a
previous complete destination on errors or interruption.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success. |
| `2` | Invalid arguments, input metadata, or output collision. |
| `3` | Missing executable, model, font, cover, or required tool capability. |
| `4` | Transcription failure or no usable timed words. |
| `5` | Media preparation, rendering, or final verification failure. |
| `130` | Interrupted by `SIGINT` or `SIGTERM`. |

## Troubleshooting

- **`missing capability subtitles`**: use an FFmpeg build with libass. On
  macOS, put `ffmpeg-full` first on `PATH` as shown above.
- **`find whisper-cli`**: install whisper.cpp, add its build directory to
  `PATH`, or pass `--whisper-bin /path/to/whisper-cli`.
- **`Whisper model is required`**: pass `--model` or configure
  `WHISPER_MODEL`.
- **No cover image**: pass `--cover artwork.png` or add embedded MP3 art.
- **Output already exists**: choose another `--output` or use `--force`.
- **Need tool details**: rerun with `--verbose`; normal progress and errors
  remain on stderr.

## Optional `WHISPER_MODEL`

Set `WHISPER_MODEL` once to omit `--model` from normal commands:

```bash
export WHISPER_MODEL="$HOME/models/ggml-medium.bin"
volnorez episode.mp3 --cover cover.jpg
```

The opt-in end-to-end test requires both `WHISPER_MODEL` and
`VOLNOREZ_E2E_MP3`; it is skipped when either is unset, and CI does not
download a model or speech fixture:

```bash
WHISPER_MODEL=/path/to/model.bin VOLNOREZ_E2E_MP3=/path/to/episode.mp3 go test ./tests -run TestPipelineWithWhisper
```
