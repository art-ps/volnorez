package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
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
	if !strings.Contains(diagnostic.String(), formatCommand(Command{
		Name: os.Args[0], Args: []string{"-test.run=TestHelperProcess", "--", "stdout", "stderr"},
	})+"\n") || !strings.Contains(diagnostic.String(), "stdout") || !strings.Contains(diagnostic.String(), "stderr") {
		t.Fatalf("diagnostic = %q", diagnostic.String())
	}
}

func TestFormatCommandQuotesSpacesCyrillicAndSpecialArguments(t *testing.T) {
	command := Command{
		Name: "/tmp/volnorez tool/волна",
		Args: []string{"обычный", "два слова", `$HOME;$(touch nope)`, `quote"and\\slash`},
	}
	want := `+ "/tmp/volnorez tool/волна" "обычный" "два слова" "$HOME;$(touch nope)" "quote\"and\\\\slash"`
	if got := formatCommand(command); got != want {
		t.Fatalf("formatCommand() = %q, want %q", got, want)
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

func TestExecRunnerCancellationTerminatesAndWaitsForRunningChild(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan int, 1)
	diagnostic := &startWriter{started: started}
	done := make(chan error, 1)
	go func() {
		_, err := (ExecRunner{Verbose: true, Diagnostic: diagnostic}).Run(ctx, Command{
			Name: os.Args[0],
			Args: []string{"-test.run=TestHelperProcess", "--", "payload", "", "wait-for-kill"},
		})
		done <- err
	}()

	select {
	case pid := <-started:
		if pid <= 0 {
			t.Fatalf("invalid child PID %d", pid)
		}
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not wait for canceled helper child to terminate")
		}
		if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
			t.Fatalf("child PID %d still exists after Run returned: %v", pid, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("helper child did not start")
	}
}

func TestStartWriterDoesNotTreatInvocationAsChildReadiness(t *testing.T) {
	started := make(chan int, 1)
	writer := &startWriter{started: started}
	invocation := formatCommand(Command{
		Name: os.Args[0], Args: []string{"-test.run=TestHelperProcess", "--", "payload", "", "wait-for-kill"},
	}) + "\n"
	if strings.Contains(invocation, childReadyPrefix) {
		t.Fatalf("child marker unexpectedly appears in invocation: %q", invocation)
	}
	if _, err := writer.Write([]byte(invocation)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
		t.Fatal("verbose invocation falsely signaled child readiness")
	default:
	}
}

func TestExecRunnerStreamsConcurrentDiagnostics(t *testing.T) {
	var diagnostic bytes.Buffer
	command := Command{
		Name: os.Args[0],
		Args: []string{"-test.run=TestHelperProcess", "--", "stdout", "stderr", "concurrent"},
	}
	result, err := (ExecRunner{Verbose: true, Diagnostic: &diagnostic}).Run(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	prefix := formatCommand(command) + "\n"
	streamed := strings.TrimSuffix(strings.TrimPrefix(diagnostic.String(), prefix), "\n")
	if len(result.Stdout) != 1024 || len(result.Stderr) != 1024 || strings.Count(streamed, "o") != 1024 || strings.Count(streamed, "e") != 1024 {
		t.Fatalf("stdout=%d stderr=%d diagnostic=%d", len(result.Stdout), len(result.Stderr), diagnostic.Len())
	}
}

func TestExecRunnerReturnsOneLineStructuredError(t *testing.T) {
	result, err := (ExecRunner{}).Run(context.Background(), Command{
		Name: os.Args[0],
		Args: []string{"-test.run=TestHelperProcess", "--", "", "first line\nsecond line", "exit"},
	})
	if err == nil || !strings.Contains(err.Error(), os.Args[0]) || !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "first line") || strings.Contains(err.Error(), "\n") {
		t.Fatalf("error is not a concise one-line summary: %q", err.Error())
	}
	if string(result.Stderr) != "first line\nsecond line" {
		t.Fatalf("result = %#v", result)
	}
	var commandErr *CommandError
	if !errors.As(fmt.Errorf("outer: %w", err), &commandErr) {
		t.Fatalf("wrapped error does not expose CommandError: %T", err)
	}
	if string(commandErr.Stderr) != string(result.Stderr) || commandErr.Command.Name != os.Args[0] {
		t.Fatalf("CommandError = %#v, result = %#v", commandErr, result)
	}
}

func TestHelperProcess(t *testing.T) {
	args := os.Args
	for i, arg := range args {
		if arg == "--" && i+2 < len(args) {
			if i+3 < len(args) && args[i+3] == "wait-for-kill" {
				_, _ = fmt.Fprintf(os.Stdout, "%s%d\n", childReadyPrefix, os.Getpid())
				ready := make(chan os.Signal, 1)
				signal.Notify(ready, os.Interrupt, syscall.SIGTERM)
				<-ready
				os.Exit(0)
			}
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

type startWriter struct {
	started chan int
	once    sync.Once
	mu      sync.Mutex
	buffer  strings.Builder
}

func (w *startWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.buffer.Write(p)
	text := w.buffer.String()
	start := strings.Index(text, childReadyPrefix)
	if start < 0 {
		return len(p), nil
	}
	value := text[start+len(childReadyPrefix):]
	end := strings.IndexByte(value, '\n')
	if end < 0 {
		return len(p), nil
	}
	pid, err := strconv.Atoi(value[:end])
	if err != nil {
		return 0, err
	}
	w.once.Do(func() { w.started <- pid })
	return len(p), nil
}

const childReadyPrefix = "child-origin-ready:"
