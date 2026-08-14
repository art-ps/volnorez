package pipeline

import (
	"os"
	"sync"
)

type Workspace struct {
	Dir        string
	once       sync.Once
	cleanupErr error
}

func NewWorkspace(parent string) (*Workspace, error) {
	dir, err := os.MkdirTemp(parent, ".volnorez-*")
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return &Workspace{Dir: dir}, nil
}

func (w *Workspace) Cleanup() error {
	w.once.Do(func() {
		w.cleanupErr = os.RemoveAll(w.Dir)
	})
	return w.cleanupErr
}
