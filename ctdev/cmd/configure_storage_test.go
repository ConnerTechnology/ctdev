package cmd

import (
	"bytes"
	"strings"
	"testing"
)

// --swapfile-size is for a boot script's batch run: without --batch, or with
// --show, it's refused before anything is detected or changed.
func TestSwapfileSizeNeedsBatch(t *testing.T) {
	for _, args := range [][]string{
		{"configure", "storage", "--swapfile-size", "2"},
		{"configure", "storage", "--show", "--swapfile-size", "2"},
		{"configure", "storage", "--batch", "--show", "--swapfile-size", "2"},
	} {
		name := strings.Join(args, " ")
		t.Run(name, func(t *testing.T) {
			reset := resetConfigureStorageFlags(t)
			reset()
			t.Cleanup(reset)
			var out bytes.Buffer
			rootCmd.SetArgs(args)
			rootCmd.SetOut(&out)
			rootCmd.SetErr(&out)
			err := rootCmd.Execute()
			rootCmd.SetArgs(nil)
			rootCmd.SetOut(nil)
			rootCmd.SetErr(nil)
			if err == nil || !strings.Contains(err.Error(), "--swapfile-size only works with --batch") {
				t.Errorf("ctdev %s: err = %v, want the --batch refusal", name, err)
			}
		})
	}
}

// resetConfigureStorageFlags returns a cleanup that puts back the flags an
// Execute of `configure storage` sets. Cobra keeps flag values and their
// Changed marks between runs of the same rootCmd.
func resetConfigureStorageFlags(t *testing.T) func() {
	t.Helper()
	storage, _, err := rootCmd.Find([]string{"configure", "storage"})
	if err != nil {
		t.Fatal(err)
	}
	return func() {
		flagBatch, flagDryRun, flagConfigShow, flagSwapfileSize = false, false, false, 0
		for _, f := range []string{"batch", "dry-run", "show", "swapfile-size"} {
			if fl := storage.Flags().Lookup(f); fl != nil {
				fl.Changed = false
			}
			if fl := rootCmd.PersistentFlags().Lookup(f); fl != nil {
				fl.Changed = false
			}
		}
	}
}

func TestSwapfileSizeBatchValidation(t *testing.T) {
	t.Run("non-positive size rejected", func(t *testing.T) {
		reset := resetConfigureStorageFlags(t)
		reset()
		t.Cleanup(reset)
		var out bytes.Buffer
		rootCmd.SetArgs([]string{"configure", "storage", "--batch", "--dry-run", "--swapfile-size", "-2"})
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		err := rootCmd.Execute()
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		if err == nil {
			t.Fatal("expected error for negative --swapfile-size")
		}
	})

	t.Run("valid positive size in batch dry-run", func(t *testing.T) {
		reset := resetConfigureStorageFlags(t)
		reset()
		t.Cleanup(reset)
		var out bytes.Buffer
		rootCmd.SetArgs([]string{"configure", "storage", "--batch", "--dry-run", "--swapfile-size", "4"})
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		var err error
		captureStdout(t, func() { err = rootCmd.Execute() })
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		if err != nil {
			t.Fatalf("unexpected error for valid --batch --dry-run --swapfile-size 4: %v", err)
		}
	})
}
