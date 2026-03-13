package shell

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

type RunOpts struct {
	Env    []string
	Dir    string
	Stdout io.Writer
	Stderr io.Writer
}

func Run(ctx context.Context, name string, args []string, opts RunOpts) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), opts.Env...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	return cmd.Run()
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return 1
}

func BoolEnv(name string, val bool) string {
	if val {
		return fmt.Sprintf("%s=true", name)
	}
	return fmt.Sprintf("%s=false", name)
}
