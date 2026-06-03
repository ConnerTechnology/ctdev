package component

import (
	"context"
	"testing"
)

func TestExecutorGoInstallUnsupportedOS(t *testing.T) {

	c := &Component{
		Name: "test-skip",
		GoInstall: func(ctx context.Context, opts ExecOpts) error {
			return ErrUnsupportedOS
		},
		GoUninstall: func(ctx context.Context, opts ExecOpts) error {
			return ErrUnsupportedOS
		},
	}

	result := Install(context.Background(), c, ExecOpts{})
	if !result.Skipped {
		t.Error("expected Skipped=true for ErrUnsupportedOS")
	}
	if result.Err != nil {
		t.Errorf("expected Err=nil for skipped, got %v", result.Err)
	}
}

func TestExecutorGoUninstallUnsupportedOS(t *testing.T) {

	c := &Component{
		Name: "test-skip",
		GoInstall: func(ctx context.Context, opts ExecOpts) error {
			return ErrUnsupportedOS
		},
		GoUninstall: func(ctx context.Context, opts ExecOpts) error {
			return ErrUnsupportedOS
		},
	}

	result := Uninstall(context.Background(), c, ExecOpts{})
	if !result.Skipped {
		t.Error("expected Skipped=true for ErrUnsupportedOS")
	}
}

func TestExecutorGoInstallSuccess(t *testing.T) {

	called := false
	c := &Component{
		Name: "test",
		GoInstall: func(ctx context.Context, opts ExecOpts) error {
			called = true
			return nil
		},
		GoUninstall: func(ctx context.Context, opts ExecOpts) error {
			return nil
		},
	}

	result := Install(context.Background(), c, ExecOpts{})
	if result.Err != nil {
		t.Fatalf("install failed: %v", result.Err)
	}
	if !called {
		t.Error("expected GoInstall to be called")
	}
}
