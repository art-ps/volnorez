package assets

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/image/font/sfnt"
)

//go:embed NotoSans.ttf
var defaultFont []byte

func PrepareFont(workspaceDir, customPath string) (fontDir, family string, err error) {
	fontDir = filepath.Join(workspaceDir, "fonts")
	if err := os.MkdirAll(fontDir, 0o700); err != nil {
		return "", "", fmt.Errorf("create font directory: %w", err)
	}

	var fontPath string
	if customPath == "" {
		fontPath = filepath.Join(fontDir, "NotoSans.ttf")
		if err := os.WriteFile(fontPath, defaultFont, 0o600); err != nil {
			return "", "", fmt.Errorf("write default font: %w", err)
		}
	} else {
		fontPath = filepath.Join(fontDir, filepath.Base(customPath))
		if err := copyFont(customPath, fontPath); err != nil {
			return "", "", err
		}
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		return "", "", fmt.Errorf("read prepared font: %w", err)
	}
	family, err = familyName(data)
	if err != nil {
		return "", "", err
	}
	return fontDir, family, nil
}

func copyFont(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open custom font: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create prepared font: %w", err)
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("copy custom font: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close prepared font: %w", closeErr)
	}
	return nil
}

func familyName(data []byte) (string, error) {
	font, err := sfnt.Parse(data)
	if err != nil {
		return "", fmt.Errorf("parse font: %w", err)
	}
	var buf sfnt.Buffer
	family, err := font.Name(&buf, sfnt.NameIDFamily)
	if err != nil {
		return "", fmt.Errorf("read font family: %w", err)
	}
	return family, nil
}
