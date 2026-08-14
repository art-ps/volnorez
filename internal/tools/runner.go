package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

type Command struct {
	Name string
	Args []string
	Dir  string
}

type Result struct {
	Stdout []byte
	Stderr []byte
}

type Runner interface {
	LookPath(string) (string, error)
	Run(context.Context, Command) (Result, error)
}

type ExecRunner struct {
	Verbose    bool
	Diagnostic io.Writer
}

func (ExecRunner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

func (r ExecRunner) Run(ctx context.Context, command Command) (Result, error) {
	cmd := exec.CommandContext(ctx, command.Name, command.Args...)
	if command.Dir != "" {
		cmd.Dir = command.Dir
	}

	var result Result
	var stdoutBuffer, stderrBuffer bytes.Buffer
	stdout := io.Writer(&stdoutBuffer)
	stderr := io.Writer(&stderrBuffer)
	if r.Verbose && r.Diagnostic != nil {
		diagnostic := lockedWriter{Writer: r.Diagnostic}
		stdout = io.MultiWriter(&stdoutBuffer, &diagnostic)
		stderr = io.MultiWriter(&stderrBuffer, &diagnostic)
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	result.Stdout = stdoutBuffer.Bytes()
	result.Stderr = stderrBuffer.Bytes()
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if err == nil {
		return result, nil
	}
	if _, ok := err.(*exec.ExitError); ok {
		return result, fmt.Errorf("%s: %w: %s", command.Name, err, strings.TrimSpace(string(result.Stderr)))
	}
	return result, fmt.Errorf("run %s: %w", command.Name, err)
}

type lockedWriter struct {
	io.Writer
	mu sync.Mutex
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Writer.Write(p)
}
