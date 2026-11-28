//go:build unix

package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// Once a command starts winding down, a second interrupt must be able to
// terminate it even if cleanup never returns.
func TestSecondInterruptTerminatesCLI(t *testing.T) {
	const helper = "CTDEV_TEST_SECOND_INTERRUPT"
	if os.Getenv(helper) == "1" {
		rootCmd.AddCommand(&cobra.Command{Use: "example-wait", Run: func(cmd *cobra.Command, _ []string) {
			fmt.Println("ready")
			<-cmd.Context().Done()
			fmt.Println("cancelled")
			select {}
		}})
		rootCmd.SetArgs([]string{"example-wait"})
		if err := Execute(); err != nil {
			t.Fatal(err)
		}
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSecondInterruptTerminatesCLI$")
	child.Env = append(os.Environ(), helper+"=1")
	child.Stderr = os.Stderr
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if child.ProcessState == nil {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	})
	sc := bufio.NewScanner(stdout)
	for _, want := range []string{"ready", "cancelled"} {
		if !sc.Scan() || sc.Text() != want {
			t.Fatalf("child output = %q (%v), want %q", sc.Text(), sc.Err(), want)
		}
		if err := child.Process.Signal(os.Interrupt); err != nil {
			t.Fatal(err)
		}
	}
	var exit *exec.ExitError
	if err := child.Wait(); !errors.As(err, &exit) || exit.Sys().(syscall.WaitStatus).Signal() != os.Interrupt {
		t.Fatalf("child exit = %v, want SIGINT", err)
	}
}
