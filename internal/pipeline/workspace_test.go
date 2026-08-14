package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceCleanupIsIdempotent(t *testing.T) {
	w, err := NewWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w.Dir, "x"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := w.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := w.Cleanup(); err != nil {
		t.Fatal(err)
	}
}

func TestNewWorkspaceIsPrivate(t *testing.T) {
	w, err := NewWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Cleanup() })
	info, err := os.Stat(w.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("mode = %o, want 700", info.Mode().Perm())
	}
}
