package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A v1 aws on PATH (apt's awscli) isn't the CLI configure aws needs: it asks
// to install v2 rather than going on to profiles or SSO.
func TestConfigureAWSOnV1OffersInstall(t *testing.T) {
	dir := t.TempDir()
	stub := "#!/bin/sh\necho 'aws-cli/1.22.34 Python/3.10.12 Linux/6.8.0 botocore/1.23.34'\n"
	if err := os.WriteFile(filepath.Join(dir, "aws"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	orig := installTool
	var installs []string
	installTool = func(_ context.Context, name string) error { installs = append(installs, name); return nil }
	t.Cleanup(func() { installTool = orig })
	useStdin(t, "n\n")

	out := captureStdout(t, func() {
		if ok, err := ensureTool(context.Background(), "aws", "AWS CLI", false); ok || err != nil {
			t.Errorf("ok=%v err=%v; a no should stop without error", ok, err)
		}
	})
	if !strings.Contains(out, "AWS CLI isn't installed. Install it now?") {
		t.Errorf("a v1 aws should get the install prompt, got:\n%s", out)
	}
	if len(installs) != 0 {
		t.Errorf("declined, but installed %v", installs)
	}
}

func TestOfferAWSConfigureSSORunsAndRereadsProfiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	useStdin(t, "\n")
	orig := runAWSConfigureSSO
	runAWSConfigureSSO = func(context.Context) error {
		dir := filepath.Join(home, ".aws")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "config"), []byte("[profile example-dev]\nsso_session = example\n"), 0o600)
	}
	t.Cleanup(func() { runAWSConfigureSSO = orig })

	var profiles []string
	var err error
	out := captureStdout(t, func() {
		profiles, err = offerAWSConfigureSSO(context.Background(), false)
	})
	if err != nil || strings.Join(profiles, ",") != "example-dev" {
		t.Errorf("profiles=%v err=%v; want [example-dev]", profiles, err)
	}
	if !strings.Contains(out, "aws configure sso") {
		t.Errorf("expected the SSO offer, got:\n%s", out)
	}
}

func TestOfferAWSConfigureSSOBatchOnlyHints(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	orig := runAWSConfigureSSO
	ran := false
	runAWSConfigureSSO = func(context.Context) error { ran = true; return nil }
	t.Cleanup(func() { runAWSConfigureSSO = orig })

	out := captureStdout(t, func() {
		if _, err := offerAWSConfigureSSO(context.Background(), true); err != nil {
			t.Error(err)
		}
	})
	if ran || !strings.Contains(out, "aws configure sso") {
		t.Errorf("batch should hint, not run SSO (ran=%v):\n%s", ran, out)
	}
}

func TestOfferAWSConfigureSSODeclined(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	useStdin(t, "n\n")
	orig := runAWSConfigureSSO
	ran := false
	runAWSConfigureSSO = func(context.Context) error { ran = true; return nil }
	t.Cleanup(func() { runAWSConfigureSSO = orig })

	captureStdout(t, func() {
		if _, err := offerAWSConfigureSSO(context.Background(), false); err != nil {
			t.Error(err)
		}
	})
	if ran {
		t.Error("a no should not run aws configure sso")
	}
}
