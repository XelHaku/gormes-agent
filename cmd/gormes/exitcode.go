package main

import (
	"errors"
	"fmt"
)

type exitCodeError struct {
	code int
	err  error
}

func newExitCodeError(code int, err error) error {
	if err == nil {
		err = fmt.Errorf("exit code %d", code)
	}
	return exitCodeError{code: code, err: err}
}

func (e exitCodeError) Error() string {
	return e.err.Error()
}

func (e exitCodeError) Unwrap() error {
	return e.err
}

func (e exitCodeError) ExitCode() int {
	return e.code
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var coded interface {
		ExitCode() int
	}
	if errors.As(err, &coded) {
		return coded.ExitCode()
	}
	return 1
}
