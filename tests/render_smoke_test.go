package tests

import (
	"context"
	"crypto/sha256"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"volnorez/internal/assets"
	"volnorez/internal/media"
	"volnorez/internal/subtitles"
	"volnorez/internal/tools"
	"volnorez/internal/transcribe"
)

func TestRenderSmoke(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not installed")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "вход с пробелом.mp3")
	cover := filepath.Join(dir, "cover.png")
	assPath := filepath.Join(dir, "subtitles.ass")
	output := filepath.Join(dir, "output.mp4")
	runExternal(t, ffmpeg, "-v", "error", "-y", "-f", "lavfi", "-i",
		"aevalsrc=sin(2*PI*(220+220*t)*t):d=3:s=44100", "-ac", "2", "-q:a", "4", input)
	runExternal(t, ffmpeg, "-v", "error", "-y", "-f", "lavfi", "-i",
		"color=c=0x8844cc:s=600x600:d=1", "-frames:v", "1", cover)

	data, err := os.ReadFile("../testdata/transcript.json")
	if err != nil {
		t.Fatal(err)
	}
	words, err := transcribe.ParseFullJSON(data, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	fontDir, family, err := assets.PrepareFont(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	assFile, err := os.Create(assPath)
	if err != nil {
		t.Fatal(err)
	}
	err = subtitles.WriteASS(assFile, subtitles.Document{
		Duration: 3 * time.Second, FontFamily: family, Accent: "#FFD84D", Phrases: subtitles.Group(words),
	})
	closeErr := assFile.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}

	runner := tools.ExecRunner{Diagnostic: io.Discard}
	err = media.Render(context.Background(), runner, media.RenderRequest{
		FFmpeg: ffmpeg, Input: input, Cover: cover, ASS: assPath, FontDir: fontDir,
		WorkDir: dir, Output: output, Accent: "#FFD84D",
		Selection: media.Selection{Duration: 3 * time.Second},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := media.VerifyOutput(context.Background(), runner, ffprobe, output, 3*time.Second); err != nil {
		t.Fatal(err)
	}

	frameA := filepath.Join(dir, "frame-a.png")
	frameB := filepath.Join(dir, "frame-b.png")
	runExternal(t, ffmpeg, "-v", "error", "-y", "-ss", "0.5", "-i", output,
		"-vf", "crop=900:420:90:760", "-frames:v", "1", frameA)
	runExternal(t, ffmpeg, "-v", "error", "-y", "-ss", "2.0", "-i", output,
		"-vf", "crop=900:420:90:760", "-frames:v", "1", frameB)
	if fileHash(t, frameA) == fileHash(t, frameB) {
		t.Fatal("waveform frames are identical")
	}
}

func TestASSLiteralPunctuationAndBackslashesRenderOnOneLine(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	fontDir, family, err := assets.PrepareFont(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	assPath := filepath.Join(dir, "literal.ass")
	assFile, err := os.Create(assPath)
	if err != nil {
		t.Fatal(err)
	}
	literal := `comma, \N \n \h {brace}`
	err = subtitles.WriteASS(assFile, subtitles.Document{
		Duration: time.Second, FontFamily: family, Accent: "#FFD84D",
		Phrases: []subtitles.Phrase{{Text: literal, Words: []transcribe.Word{{Text: literal, End: time.Second}}}},
	})
	closeErr := assFile.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}

	frame := filepath.Join(dir, "literal.png")
	cmd := exec.Command(ffmpeg,
		"-v", "error", "-y", "-f", "lavfi", "-i", "color=c=black:s=1080x1920:d=1:r=1",
		"-vf", "subtitles=literal.ass:fontsdir="+filepath.Base(fontDir), "-frames:v", "1", frame,
	)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("render literal ASS: %v\n%s", err, out)
	}
	bounds := nonBlackBounds(t, frame)
	if bounds.Empty() {
		t.Fatal("literal subtitle did not render")
	}
	if bounds.Dy() > 100 {
		t.Fatalf("literal control-looking text rendered as multiple lines: bounds=%v", bounds)
	}
}

func runExternal(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v\n%s", name, err, out)
	}
}

func fileHash(t *testing.T, path string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

func nonBlackBounds(t *testing.T, path string) image.Rectangle {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	img, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	bounds := img.Bounds()
	result := image.Rectangle{Min: bounds.Max, Max: bounds.Min}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r == 0 && g == 0 && b == 0 {
				continue
			}
			if x < result.Min.X {
				result.Min.X = x
			}
			if y < result.Min.Y {
				result.Min.Y = y
			}
			if x+1 > result.Max.X {
				result.Max.X = x + 1
			}
			if y+1 > result.Max.Y {
				result.Max.Y = y + 1
			}
		}
	}
	if result.Min.X > result.Max.X || result.Min.Y > result.Max.Y {
		return image.Rectangle{}
	}
	return result
}
