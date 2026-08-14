package tools

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestExecRunnerCapturesAndStreamsDiagnostics(t *testing.T) {
	var diagnostic bytes.Buffer
	runner := ExecRunner{Verbose: true, Diagnostic: &diagnostic}
	result, err := runner.Run(context.Background(), Command{
		Name: os.Args[0],
		Args: []string{"-test.run=TestHelperProcess", "--", "stdout", "stderr"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Stdout) != "stdout" || string(result.Stderr) != "stderr" {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(diagnostic.String(), "stdout") || !strings.Contains(diagnostic.String(), "stderr") {
		t.Fatalf("diagnostic = %q", diagnostic.String())
	}
}

func TestExecRunnerReturnsContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (ExecRunner{}).Run(ctx, Command{Name: os.Args[0], Args: []string{"-test.run=TestHelperProcess"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestExecRunnerStreamsConcurrentDiagnostics(t *testing.T) {
	var diagnostic bytes.Buffer
	result, err := (ExecRunner{Verbose: true, Diagnostic: &diagnostic}).Run(context.Background(), Command{
		Name: os.Args[0],
		Args: []string{"-test.run=TestHelperProcess", "--", "stdout", "stderr", "concurrent"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Stdout) != 1024 || len(result.Stderr) != 1024 || diagnostic.Len() != 2048 {
		t.Fatalf("stdout=%d stderr=%d diagnostic=%d", len(result.Stdout), len(result.Stderr), diagnostic.Len())
	}
}

func TestExecRunnerIncludesExitStatusAndStderr(t *testing.T) {
	result, err := (ExecRunner{}).Run(context.Background(), Command{
		Name: os.Args[0],
		Args: []string{"-test.run=TestHelperProcess", "--", "", "failure", "exit"},
	})
	if err == nil || !strings.Contains(err.Error(), os.Args[0]) || !strings.Contains(err.Error(), "exit status 1") || !strings.Contains(err.Error(), "failure") {
		t.Fatalf("error = %v", err)
	}
	if string(result.Stderr) != "failure" {
		t.Fatalf("result = %#v", result)
	}
}

func TestHelperProcess(t *testing.T) {
	args := os.Args
	for i, arg := range args {
		if arg == "--" && i+2 < len(args) {
			if i+3 < len(args) && args[i+3] == "concurrent" {
				var wg sync.WaitGroup
				wg.Add(2)
				go func() {
					defer wg.Done()
					_, _ = os.Stdout.WriteString(strings.Repeat("o", 1024))
				}()
				go func() {
					defer wg.Done()
					_, _ = os.Stderr.WriteString(strings.Repeat("e", 1024))
				}()
				wg.Wait()
				os.Exit(0)
			}
			_, _ = os.Stdout.WriteString(args[i+1])
			_, _ = os.Stderr.WriteString(args[i+2])
			if i+3 < len(args) && args[i+3] == "exit" {
				os.Exit(1)
			}
			os.Exit(0)
		}
	}
	return
}
