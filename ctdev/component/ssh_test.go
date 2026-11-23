package component

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

type fakeSSH struct {
	calls         [][]string
	remoteLoginOn bool
	sudoErr       error
}

func useFakeSSH(t *testing.T) *fakeSSH {
	t.Helper()
	f := &fakeSSH{}
	origSudo, origEnsure, origDisable := sshSudo, sshEnsureLinux, sshDisableLinux
	origRemove, origLogin := sshRemovePackage, sshRemoteLoginOn
	sshSudo = func(_ context.Context, _ sysutil.Opts, name string, args ...string) error {
		f.calls = append(f.calls, append([]string{"sudo", name}, args...))
		return f.sudoErr
	}
	sshEnsureLinux = func(context.Context, sysutil.Opts) error {
		f.calls = append(f.calls, []string{"ensure linux ssh server"})
		return nil
	}
	sshDisableLinux = func(context.Context, sysutil.Opts) error {
		f.calls = append(f.calls, []string{"disable linux ssh server"})
		return nil
	}
	sshRemovePackage = func(_ context.Context, _ sysutil.Opts, pkgs ...string) error {
		f.calls = append(f.calls, append([]string{"remove"}, pkgs...))
		return nil
	}
	sshRemoteLoginOn = func(context.Context) bool { return f.remoteLoginOn }
	t.Cleanup(func() {
		sshSudo, sshEnsureLinux, sshDisableLinux = origSudo, origEnsure, origDisable
		sshRemovePackage, sshRemoteLoginOn = origRemove, origLogin
	})
	return f
}

var quietExec = ExecOpts{Stdout: io.Discard, Stderr: io.Discard}

func TestSSHInstall(t *testing.T) {
	tests := []struct {
		name          string
		pm            string
		remoteLoginOn bool
		force         bool
		want          [][]string
	}{
		{"linux sets up the server", "apt", false, false, [][]string{{"ensure linux ssh server"}}},
		{"mac turns Remote Login on", "brew", false, false, [][]string{{"sudo", "systemsetup", "-setremotelogin", "on"}}},
		{"mac already on", "brew", true, false, nil},
		{"mac already on, forced", "brew", true, true, [][]string{{"sudo", "systemsetup", "-setremotelogin", "on"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := useFakeSSH(t)
			f.remoteLoginOn = tt.remoteLoginOn
			opts := quietExec
			opts.Force = tt.force
			if err := sshInstallWith(context.Background(), opts, tt.pm); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(f.calls, tt.want) {
				t.Errorf("calls = %q, want %q", f.calls, tt.want)
			}
		})
	}
}

func TestSSHInstallMacRefusalPointsAtSystemSettings(t *testing.T) {
	f := useFakeSSH(t)
	f.sudoErr = errors.New("exit status 1")
	err := sshInstallWith(context.Background(), quietExec, "brew")
	if err == nil || !strings.Contains(err.Error(), "System Settings > General > Sharing > Remote Login") {
		t.Errorf("err = %v, want one naming the System Settings switch", err)
	}
}

func TestSSHInstallUnsupportedPM(t *testing.T) {
	useFakeSSH(t)
	if err := sshInstallWith(context.Background(), quietExec, "dnf"); err == nil {
		t.Error("want an error on a package manager ssh doesn't support")
	}
}

func TestSSHUninstall(t *testing.T) {
	tests := []struct {
		pm   string
		want [][]string
	}{
		{"apt", [][]string{{"disable linux ssh server"}, {"remove", "openssh-server"}}},
		{"brew", [][]string{{"sudo", "systemsetup", "-f", "-setremotelogin", "off"}}},
	}
	for _, tt := range tests {
		t.Run(tt.pm, func(t *testing.T) {
			f := useFakeSSH(t)
			if err := sshUninstallWith(context.Background(), quietExec, tt.pm); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(f.calls, tt.want) {
				t.Errorf("calls = %q, want %q", f.calls, tt.want)
			}
		})
	}
}
