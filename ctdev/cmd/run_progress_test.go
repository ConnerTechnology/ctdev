package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	comp "github.com/ConnerTechnology/ctdev/ctdev/component"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/progress"
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
		mode: mode,
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

// registerDepChain registers dep-base (whose install and uninstall both fail),
// dep-mid (depends on dep-base) and dep-top (depends on dep-mid), recording
// which ones actually ran.
func registerDepChain(t *testing.T) map[string]bool {
	t.Helper()
	ran := map[string]bool{}
	step := func(name string, err error) func(context.Context, comp.ExecOpts) error {
		return func(context.Context, comp.ExecOpts) error {
			ran[name] = true
			return err
		}
	}
	boom := errors.New("boom")
	comp.RegisterForTest(t, comp.Component{
		Name: "dep-base", GoInstall: step("dep-base", boom), GoUninstall: step("dep-base", boom),
	})
	comp.RegisterForTest(t, comp.Component{
		Name: "dep-mid", Dependencies: []string{"dep-base"},
		GoInstall: step("dep-mid", nil), GoUninstall: step("dep-mid", nil),
	})
	comp.RegisterForTest(t, comp.Component{
		Name: "dep-top", Dependencies: []string{"dep-mid"},
		GoInstall: step("dep-top", nil), GoUninstall: step("dep-top", nil),
	})
	return ran
}

var depChain = []string{"dep-base", "dep-mid", "dep-top"}

func TestRunComponents_SkipsDependentOfFailedDependency(t *testing.T) {
	ran := registerDepChain(t)

	sender := &captureSender{}
	runComponents(context.Background(), sender, progressOperation{mode: progress.ModeInstall, names: depChain})

	if ran["dep-mid"] || ran["dep-top"] {
		t.Errorf("dependents of a failed dependency ran: %v", ran)
	}
	blocked := map[string]string{}
	for _, m := range sender.Messages() {
		if b, ok := m.(progress.InstallBlockedMsg); ok {
			blocked[b.Name] = b.Reason
		}
	}
	if blocked["dep-mid"] != "dep-base failed" {
		t.Errorf("dep-mid reason = %q, want %q", blocked["dep-mid"], "dep-base failed")
	}
	if blocked["dep-top"] != "dep-mid skipped" {
		t.Errorf("dep-top reason = %q, want %q", blocked["dep-top"], "dep-mid skipped")
	}
}

// Uninstall runs every selected component: a dependency failing to uninstall
// is no reason to keep its dependents.
func TestRunComponents_UninstallIgnoresDependencies(t *testing.T) {
	ran := registerDepChain(t)

	sender := &captureSender{}
	runComponents(context.Background(), sender, progressOperation{mode: progress.ModeUninstall, names: depChain})

	if !ran["dep-mid"] || !ran["dep-top"] {
		t.Errorf("uninstall should run every selected component; ran: %v", ran)
	}
}

func TestRunWithProgressBatch_SkipsDependentOfFailedDependency(t *testing.T) {
	ran := registerDepChain(t)

	var err error
	out := captureStdout(t, func() {
		err = runWithProgressBatch(context.Background(), progressOperation{mode: progress.ModeInstall, names: depChain})
	})

	if ran["dep-mid"] || ran["dep-top"] {
		t.Errorf("dependents of a failed dependency ran: %v", ran)
	}
	for _, want := range []string{"dep-mid skipped: dep-base failed", "dep-top skipped: dep-mid skipped"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in batch output, got:\n%s", want, out)
		}
	}
	// One real failure; the skipped dependents are not counted as failures.
	if err == nil || err.Error() != "1 component(s) failed" {
		t.Errorf("err = %v, want \"1 component(s) failed\"", err)
	}
}
