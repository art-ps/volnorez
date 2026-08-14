package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"volnorez/internal/cli"
	"volnorez/internal/pipeline"
	"volnorez/internal/tools"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	return runWith(args, getenv, stdout, stderr, defaultAppDeps())
}

type appDeps struct {
	context      func() (context.Context, context.CancelFunc)
	pipelineDeps func(tools.Runner) pipeline.Dependencies
	pipeline     func(context.Context, cli.Config, pipeline.Dependencies, io.Writer) (string, error)
}

func defaultAppDeps() appDeps {
	return appDeps{
		context: func() (context.Context, context.CancelFunc) {
			return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		},
		pipelineDeps: pipeline.DefaultDependencies,
		pipeline:     pipeline.Run,
	}
}

func runWith(args []string, getenv func(string) string, stdout, stderr io.Writer, deps appDeps) int {
	cfg, err := cli.Parse(args, getenv)
	if errors.Is(err, flag.ErrHelp) {
		_, _ = io.WriteString(stdout, cli.Usage())
		return 0
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "volnorez:", oneLineError(err))
		return cli.Code(err)
	}

	ctx, stop := deps.context()
	defer stop()
	runner := tools.ExecRunner{Verbose: cfg.Verbose, Diagnostic: stderr}
	output, err := deps.pipeline(ctx, cfg, deps.pipelineDeps(runner), stderr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "volnorez:", oneLineError(err))
		return pipeline.Code(err)
	}
	_, _ = fmt.Fprintln(stdout, output)
	return 0
}

func oneLineError(err error) string {
	return strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(err.Error())
}
