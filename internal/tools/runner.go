package tools

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"strconv"
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

type CommandError struct {
	Command        Command
	Err            error
	Stdout, Stderr []byte
}

func (e *CommandError) Error() string {
	return e.Command.Name + ": " + strings.Join(strings.Fields(e.Err.Error()), " ")
}

func (e *CommandError) Unwrap() error {
	return e.Err
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
	var diagnostic *diagnosticWriter
	if r.Verbose && r.Diagnostic != nil {
		diagnostic = &diagnosticWriter{Writer: r.Diagnostic}
		_, _ = io.WriteString(diagnostic, formatCommand(command)+"\n")
		stdout = io.MultiWriter(&stdoutBuffer, diagnostic)
		stderr = io.MultiWriter(&stderrBuffer, diagnostic)
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if diagnostic != nil {
		diagnostic.ensureNewline()
	}
	result.Stdout = stdoutBuffer.Bytes()
	result.Stderr = stderrBuffer.Bytes()
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if err == nil {
		return result, nil
	}
	return result, &CommandError{
		Command: command, Err: err,
		Stdout: append([]byte(nil), result.Stdout...), Stderr: append([]byte(nil), result.Stderr...),
	}
}

func formatCommand(command Command) string {
	quoted := make([]string, 0, len(command.Args)+1)
	quoted = append(quoted, strconv.Quote(command.Name))
	for _, arg := range command.Args {
		quoted = append(quoted, strconv.Quote(arg))
	}
	return "+ " + strings.Join(quoted, " ")
}

type diagnosticWriter struct {
	io.Writer
	mu      sync.Mutex
	last    byte
	written bool
}

func (w *diagnosticWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.Writer.Write(p)
	if n > 0 {
		w.last = p[n-1]
		w.written = true
	}
	return n, err
}

func (w *diagnosticWriter) ensureNewline() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.written || w.last == '\n' {
		return
	}
	_, _ = io.WriteString(w.Writer, "\n")
	w.last = '\n'
}
