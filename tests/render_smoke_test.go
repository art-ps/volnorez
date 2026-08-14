package tests

import (
	"context"
	"crypto/sha256"
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
