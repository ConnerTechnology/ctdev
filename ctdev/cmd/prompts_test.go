package cmd

import (
	"bufio"
	"context"
	"errors"
	"strings"
	"testing"
)

// useStdin feeds prompts the given raw input and restores stdinScanner on cleanup.
func useStdin(t *testing.T, input string) {
	t.Helper()
	saved := stdinScanner
	stdinScanner = bufio.NewScanner(strings.NewReader(input))
	t.Cleanup(func() { stdinScanner = saved })
}

// typeLines feeds the line prompts the given answers, one per line.
func typeLines(t *testing.T, lines ...string) {
	t.Helper()
	useStdin(t, strings.Join(lines, "\n")+"\n")
}

func TestPromptYesNoCtx(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name, in string
		def      bool
		want     bool
		wantErr  error
	}{
		{"empty takes true default", "\n", true, true, nil},
		{"empty takes false default", "\n", false, false, nil},
		{"yes overrides false", "yes\n", false, true, nil},
		{"n overrides true", "N\n", true, false, nil},
		{"junk falls back to def", "maybe\n", true, true, nil},
		{"eof cancels", "", true, false, errPromptCancelled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			useStdin(t, tc.in)
			var got bool
			var err error
			captureStdout(t, func() { got, err = promptYesNoCtx(ctx, "Continue?", tc.def) })
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPromptChoiceCtx(t *testing.T) {
	ctx := context.Background()
	useStdin(t, "3\n\nabc\n")
	if got, err := promptChoiceCtx(ctx, 1); err != nil || got != 3 {
		t.Errorf("numeric choice: got %d, %v", got, err)
	}
	if got, err := promptChoiceCtx(ctx, 2); err != nil || got != 2 {
		t.Errorf("empty choice: got %d, %v", got, err)
	}
	if got, err := promptChoiceCtx(ctx, 4); err != nil || got != 4 {
		t.Errorf("invalid choice: got %d, %v", got, err)
	}
	if _, err := promptChoiceCtx(ctx, 1); !errors.Is(err, errPromptCancelled) {
		t.Errorf("EOF choice: err = %v, want errPromptCancelled", err)
	}
}

func TestPromptSecretAndRequiredCtx(t *testing.T) {
	ctx := context.Background()

	// Blank lines re-prompt until a non-empty secret is entered.
	useStdin(t, "\n\nmy-token\n")
	var got string
	var err error
	out := captureStdout(t, func() { got, err = promptSecretRequiredCtx(ctx, "Token", "") })
	if err != nil || got != "my-token" {
		t.Fatalf("promptSecretRequiredCtx = %q, %v; want my-token", got, err)
	}
	if !strings.Contains(out, "required") {
		t.Errorf("expected re-prompt warning on blank input, got %q", out)
	}

	// Existing secret is kept on empty Enter and never echoed.
	useStdin(t, "\n")
	out = captureStdout(t, func() { got, err = promptSecretRequiredCtx(ctx, "Token", "existing-secret") })
	if err != nil || got != "existing-secret" {
		t.Fatalf("keep existing = %q, %v", got, err)
	}
	if strings.Contains(out, "existing-secret") {
		t.Error("promptSecretRequiredCtx echoed existing secret")
	}

	// promptRequiredCtx re-prompts on blank then accepts value.
	useStdin(t, "\nexample-host\n")
	captureStdout(t, func() { got, err = promptRequiredCtx(ctx, "Host", "") })
	if err != nil || got != "example-host" {
		t.Fatalf("promptRequiredCtx = %q, %v", got, err)
	}

	// cancelToClean turns errPromptCancelled into nil and preserves other errors.
	captureStdout(t, func() {
		if err := cancelToClean(errPromptCancelled); err != nil {
			t.Errorf("cancelToClean(cancelled) = %v, want nil", err)
		}
	})
	other := errors.New("real failure")
	if err := cancelToClean(other); !errors.Is(err, other) {
		t.Errorf("cancelToClean(other) = %v, want %v", err, other)
	}
}
