package cmd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	comp "github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/progress"
)

// captureSender records the progress messages runOneComponent emits so tests
// can assert on the sequence without spinning up a real Bubble Tea program.
type captureSender struct {
	mu   sync.Mutex
	msgs []tea.Msg
}

func (inst *captureSender) Send(m tea.Msg) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	inst.msgs = append(inst.msgs, m)
}

func (inst *captureSender) Messages() []tea.Msg {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	out := make([]tea.Msg, len(inst.msgs))
	copy(out, inst.msgs)
	return out
}

func newTestOp(t *testing.T, mode progress.Mode) progressOperation {
	t.Helper()
	return progressOperation{
		mode:     mode,
		executor: comp.NewExecutor(),
	}
}

func TestRunOneComponent_DoneSendsStartAndDone(t *testing.T) {
	comp.RegisterForTest(t, comp.Component{
		Name: "rc-done",
		GoInstall: func(ctx context.Context, opts comp.ExecOpts) error {
			fmt.Fprintln(opts.Stdout, "installing")
			return nil
		},
		GoUninstall: func(ctx context.Context, opts comp.ExecOpts) error { return nil },
	})

	sender := &captureSender{}
	runOneComponent(context.Background(), sender, newTestOp(t, progress.ModeInstall), "rc-done")

	msgs := sender.Messages()
	if _, ok := msgs[0].(progress.InstallStartMsg); !ok {
		t.Errorf("expected first msg InstallStartMsg; got %T", msgs[0])
	}
	sawOutput := false
	for _, m := range msgs {
		if out, ok := m.(progress.InstallOutputMsg); ok && out.Line == "installing" {
			sawOutput = true
		}
	}
	if !sawOutput {
		t.Error("expected InstallOutputMsg for 'installing' line")
	}
	last := msgs[len(msgs)-1]
	if _, ok := last.(progress.InstallDoneMsg); !ok {
		t.Errorf("expected last msg InstallDoneMsg; got %T", last)
	}
}

func TestRunOneComponent_FailPreservesError(t *testing.T) {
	comp.RegisterForTest(t, comp.Component{
		Name: "rc-fail",
		GoInstall: func(ctx context.Context, opts comp.ExecOpts) error {
			return errors.New("boom")
		},
		GoUninstall: func(ctx context.Context, opts comp.ExecOpts) error { return nil },
	})

	sender := &captureSender{}
	runOneComponent(context.Background(), sender, newTestOp(t, progress.ModeInstall), "rc-fail")

	msgs := sender.Messages()
	last := msgs[len(msgs)-1]
	fail, ok := last.(progress.InstallFailMsg)
	if !ok {
		t.Fatalf("expected last msg InstallFailMsg; got %T", last)
	}
	if fail.Error != "boom" {
		t.Errorf("error text = %q, want %q", fail.Error, "boom")
	}
}

func TestRunOneComponent_SkipOnUnsupportedOS(t *testing.T) {
	comp.RegisterForTest(t, comp.Component{
		Name: "rc-skip",
		GoInstall: func(ctx context.Context, opts comp.ExecOpts) error {
			return comp.ErrUnsupportedOS
		},
		GoUninstall: func(ctx context.Context, opts comp.ExecOpts) error { return nil },
	})

	sender := &captureSender{}
	runOneComponent(context.Background(), sender, newTestOp(t, progress.ModeInstall), "rc-skip")

	msgs := sender.Messages()
	last := msgs[len(msgs)-1]
	if _, ok := last.(progress.InstallSkipMsg); !ok {
		t.Errorf("expected last msg InstallSkipMsg; got %T", last)
	}
}

func TestRunOneComponent_UnknownNameIsNoop(t *testing.T) {
	sender := &captureSender{}
	runOneComponent(context.Background(), sender, newTestOp(t, progress.ModeInstall), "does-not-exist-xyz")
	if n := len(sender.Messages()); n != 0 {
		t.Errorf("expected no messages for unknown name, got %d", n)
	}
}

// A long-running install that exits as soon as ctx is canceled should still
// reach the "done/fail" path — runOneComponent must wait for the scanner
// goroutine instead of leaking it.
func TestRunOneComponent_ScannerDrainsBeforeReturn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	released := make(chan struct{})
	comp.RegisterForTest(t, comp.Component{
		Name: "rc-drain",
		GoInstall: func(ctx context.Context, opts comp.ExecOpts) error {
			fmt.Fprintln(opts.Stdout, "line1")
			fmt.Fprintln(opts.Stdout, "line2")
			fmt.Fprintln(opts.Stdout, "line3")
			<-released
			return nil
		},
		GoUninstall: func(ctx context.Context, opts comp.ExecOpts) error { return nil },
	})

	sender := &captureSender{}
	done := make(chan struct{})
	go func() {
		runOneComponent(ctx, sender, newTestOp(t, progress.ModeInstall), "rc-drain")
		close(done)
	}()

	// Let the install goroutine publish its lines then cancel.
	close(released)
	cancel()
	<-done

	seen := map[string]bool{}
	for _, m := range sender.Messages() {
		if out, ok := m.(progress.InstallOutputMsg); ok {
			seen[out.Line] = true
		}
	}
	for _, want := range []string{"line1", "line2", "line3"} {
		if !seen[want] {
			t.Errorf("expected to see %q streamed through scanner", want)
		}
	}
}
