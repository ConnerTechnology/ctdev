package backup

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// ErrWrongPassword means restic found no key the password opens: a wrong
// master password or Secret Key, or a computer key that has been removed.
var ErrWrongPassword = errors.New("wrong master password or Secret Key")

// ErrDestinationBusy means restic couldn't lock the destination within its
// --retry-lock wait: another computer holds the lock, usually for the
// weekly maintenance's prune or check.
var ErrDestinationBusy = errors.New("the destination is busy: another computer holds its lock")

// ErrNoRestic means the restic binary isn't installed.
var ErrNoRestic = errors.New("restic isn't installed: run ctdev install restic")

// retryLock is how long an operation waits for a lock held by another
// computer, such as the weekly data check, before failing (CTD-87).
const retryLock = "30m"

// run is one restic invocation against a destination.
type run struct {
	password    string // reaches restic as RESTIC_PASSWORD, never argv
	newPassword string // for key add: handed over on fd 3, never argv
	keyHint     string // the key to try first; restic otherwise tries each in turn
	dir         string // working directory
	args        []string
	stdin       []byte    // what restic reads on stdin, such as backup --stdin's file
	stdout      io.Writer // where restic's output goes instead of being returned, such as a dump
	stderr      io.Writer // also gets restic's stderr, for a caller that reads more than the error
}

// restic runs restic as the user (never through sudo) and returns its
// stdout. Secrets travel in the environment and on a pipe, so they never show
// in argv, where any user on the machine could read them in ps. That is also
// why this runs exec directly: sysutil.Run passes no environment or pipe.
func (d *Destination) restic(ctx context.Context, r run) ([]byte, error) {
	bin, err := checkRestic(ctx, d.m.resticBin())
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, bin, r.args...)
	cmd.Dir = r.dir
	cmd.Env = d.env(r.password, r.keyHint)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if r.stdout != nil {
		cmd.Stdout = r.stdout
	}
	if r.stderr != nil {
		cmd.Stderr = io.MultiWriter(&stderr, r.stderr)
	}
	if r.stdin != nil {
		cmd.Stdin = bytes.NewReader(r.stdin)
	}
	if r.newPassword != "" {
		pr, pw, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		defer pr.Close()
		cmd.ExtraFiles = []*os.File{pr} // fd 3 in the child
		go func() {
			_, _ = pw.WriteString(r.newPassword)
			pw.Close()
		}()
	}
	err = cmd.Run()
	if err == nil {
		return stdout.Bytes(), nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		switch exit.ExitCode() {
		case 12: // restic: wrong password or no key found
			return stdout.Bytes(), ErrWrongPassword
		case 3: // restic backup: snapshot made, but some files couldn't be read
			return stdout.Bytes(), errPartial
		case 11: // restic: couldn't lock the repository, even after --retry-lock
			return stdout.Bytes(), ErrDestinationBusy
		}
	}
	return stdout.Bytes(), fmt.Errorf("restic %s: %s", r.args[0], resticMessage(stderr.Bytes(), err))
}

// minRestic is the oldest restic whose JSON output ctdev reads: 0.17 added
// the summary to `snapshots --json` and the backup's start and end times.
// Ubuntu 22.04 and 24.04 ship older ones in apt; ctdev install restic installs 0.19.1.
var minRestic = [3]int{0, 17, 0}

// resticChecked caches the version check per resolved binary for the process.
var resticChecked sync.Map

// checkRestic finds the binary and makes sure it's new enough, returning its
// path. The lookup isn't cached, so installing restic mid-run is noticed.
func checkRestic(ctx context.Context, bin string) (string, error) {
	path, err := exec.LookPath(bin)
	if err != nil {
		return "", ErrNoRestic
	}
	if cached, ok := resticChecked.Load(path); ok {
		if cached == nil {
			return path, nil
		}
		return "", cached.(error)
	}
	out, err := exec.CommandContext(ctx, path, "version").Output()
	if err != nil {
		return "", fmt.Errorf("run restic version: %w", err)
	}
	if err := resticVersionOK(string(out)); err != nil {
		resticChecked.Store(path, err)
		return "", err
	}
	resticChecked.Store(path, nil)
	return path, nil
}

var resticVersion = regexp.MustCompile(`^restic (\d+)\.(\d+)\.(\d+)`)

// resticVersionOK reads `restic version` output.
func resticVersionOK(out string) error {
	m := resticVersion.FindStringSubmatch(strings.TrimSpace(out))
	if m == nil {
		return fmt.Errorf("can't read restic's version from %q", strings.TrimSpace(out))
	}
	var v [3]int
	for i := range v {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	for i := range v {
		if v[i] != minRestic[i] {
			if v[i] < minRestic[i] {
				return fmt.Errorf("restic %d.%d.%d is too old: ctdev needs %d.%d or newer: run ctdev install restic",
					v[0], v[1], v[2], minRestic[0], minRestic[1])
			}
			break
		}
	}
	return nil
}

// errPartial is restic backup's "snapshot created, some files unreadable".
var errPartial = errors.New("some files couldn't be read")

// resticMessage pulls the error out of restic's stderr: the exit_error
// message in --json mode, or the last line otherwise.
func resticMessage(stderr []byte, err error) string {
	var last string
	sc := bufio.NewScanner(bytes.NewReader(stderr))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var msg struct {
			Type    string `json:"message_type"`
			Message string `json:"message"`
		}
		if json.Unmarshal([]byte(line), &msg) == nil && msg.Type == "exit_error" {
			return msg.Message
		}
		last = line
	}
	if last == "" {
		return err.Error()
	}
	return last
}

// scrubbedEnv names variables dropped from the inherited environment, so a
// RESTIC_PASSWORD or AWS key in the user's shell can't redirect a run to
// another repository or key.
var scrubbedEnv = []string{
	"RESTIC_REPOSITORY", "RESTIC_REPOSITORY_FILE",
	"RESTIC_PASSWORD", "RESTIC_PASSWORD_FILE", "RESTIC_PASSWORD_COMMAND",
	"RESTIC_KEY_HINT", "RESTIC_HOST",
	"B2_ACCOUNT_ID", "B2_ACCOUNT_KEY",
	"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE",
}

func (d *Destination) env(password, keyHint string) []string {
	drop := map[string]bool{}
	for _, k := range scrubbedEnv {
		drop[k] = true
	}
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if !drop[k] {
			env = append(env, kv)
		}
	}
	env = append(env, "RESTIC_REPOSITORY="+d.Address, "RESTIC_PASSWORD="+password)
	if keyHint != "" {
		// Each key restic tries costs a deliberately slow scrypt, so name ours.
		env = append(env, "RESTIC_KEY_HINT="+keyHint)
	}
	for k, v := range d.credentials {
		env = append(env, k+"="+v)
	}
	return env
}

// asComputer is a run with this computer's own key.
func (d *Destination) asComputer(dir string, args ...string) run {
	return run{password: d.computerKey, keyHint: d.computerKeyID, dir: dir, args: args}
}

// dryRun prints a restic command instead of running it. The command line has
// no secrets in it, so it's safe to show.
func (d *Destination) dryRun(r run) {
	fmt.Fprintf(d.m.out(), "[dry-run] restic %s\n", strings.Join(quoteArgs(r.args), " "))
}

func quoteArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t'\"") {
			a = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
		out[i] = a
	}
	return out
}
