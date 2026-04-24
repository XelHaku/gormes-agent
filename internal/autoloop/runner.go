package autoloop

import (
	"bytes"
	"context"
	"os/exec"
)

type Command struct {
	Name string
	Args []string
	Dir  string
	Env  []string
}

type Result struct {
	Stdout string
	Stderr string
	Err    error
}

type Runner interface {
	Run(context.Context, Command) Result
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, command Command) Result {
	cmd := exec.CommandContext(ctx, command.Name, command.Args...)
	cmd.Dir = command.Dir
	if command.Env != nil {
		cmd.Env = command.Env
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	return Result{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
		Err:    err,
	}
}

type FakeRunner struct {
	Commands []Command
	Results  []Result
}

func (runner *FakeRunner) Run(_ context.Context, command Command) Result {
	runner.Commands = append(runner.Commands, command)
	if len(runner.Results) == 0 {
		return Result{}
	}

	result := runner.Results[0]
	runner.Results = runner.Results[1:]
	return result
}
