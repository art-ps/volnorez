package assets

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font/sfnt"
)

func TestPrepareFontExtractsDefaultNotoSans(t *testing.T) {
	workspace := t.TempDir()
	fontDir, family, err := PrepareFont(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if fontDir != filepath.Join(workspace, "fonts") {
		t.Fatalf("fontDir = %q", fontDir)
	}
	if family != "Noto Sans" {
		t.Fatalf("family = %q, want Noto Sans", family)
	}
	data, err := os.ReadFile(filepath.Join(fontDir, "NotoSans.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("default font is empty")
	}
	font, err := sfnt.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var buf sfnt.Buffer
	name, err := font.Name(&buf, sfnt.NameIDFamily)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Noto Sans" {
		t.Fatalf("embedded family = %q, want Noto Sans", name)
	}
}

func TestPrepareFontCopiesCustomFontAndReadsFamily(t *testing.T) {
	workspace := t.TempDir()
	customPath := filepath.Join(t.TempDir(), "custom.ttf")
	if err := os.WriteFile(customPath, defaultFont, 0o600); err != nil {
		t.Fatal(err)
	}

	fontDir, family, err := PrepareFont(workspace, customPath)
	if err != nil {
		t.Fatal(err)
	}
	if family != "Noto Sans" {
		t.Fatalf("family = %q, want Noto Sans", family)
	}
	copied, err := os.ReadFile(filepath.Join(fontDir, filepath.Base(customPath)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(copied, defaultFont) {
		t.Fatal("custom font was not copied unchanged")
	}
	font, err := sfnt.Parse(copied)
	if err != nil {
		t.Fatal(err)
	}
	var buf sfnt.Buffer
	name, err := font.Name(&buf, sfnt.NameIDFamily)
	if err != nil {
		t.Fatal(err)
	}
	if name != family {
		t.Fatalf("copied family = %q, returned %q", name, family)
	}
}
