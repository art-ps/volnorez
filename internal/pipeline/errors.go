package pipeline

import (
	"context"
	"errors"
)

type Error struct {
	Code int
	Op   string
	Err  error
}

func (e *Error) Error() string {
	return e.Op + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error {
	return e.Err
}

func Code(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, context.Canceled) {
		return 130
	}
	var pipelineError *Error
	if errors.As(err, &pipelineError) {
		return pipelineError.Code
	}
	return 5
}
