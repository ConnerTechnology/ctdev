package gpu

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSignNvidiaModule_PlainZstXz(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Linux kernel module signing")
	}
	ctx := context.Background()

	tests := []struct {
		name      string
		module    string
		wantLines []string
	}{
		{
			name:      "plain .ko",
			module:    "/lib/modules/test/nvidia.ko",
			wantLines: []string{"sign-file sha256 " + MOKPriv + " " + MOKCert + " /lib/modules/test/nvidia.ko"},
		},
		{
			name:   "zstd .ko.zst",
			module: "/lib/modules/test/nvidia.ko.zst",
			wantLines: []string{
				"zstd -d -f /lib/modules/test/nvidia.ko.zst -o /lib/modules/test/nvidia.ko",
				"sign-file sha256 " + MOKPriv + " " + MOKCert + " /lib/modules/test/nvidia.ko",
				"zstd -f /lib/modules/test/nvidia.ko -o /lib/modules/test/nvidia.ko.zst",
				"rm -f /lib/modules/test/nvidia.ko",
			},
		},
		{
			name:   "xz .ko.xz",
			module: "/lib/modules/test/nvidia.ko.xz",
			wantLines: []string{
				"xz -d -k -f /lib/modules/test/nvidia.ko.xz",
				"sign-file sha256 " + MOKPriv + " " + MOKCert + " /lib/modules/test/nvidia.ko",
				"xz -f /lib/modules/test/nvidia.ko",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := signNvidiaModule(ctx, Opts{Stdout: &out, DryRun: true}, "sign-file", tt.module)
			if err != nil {
				t.Fatalf("signNvidiaModule(%s): %v", tt.module, err)
			}
			got := out.String()
			for _, want := range tt.wantLines {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q:\n%s", want, got)
				}
			}
		})
	}
}

func TestSudoTeeWriteAndAppend_DryRun(t *testing.T) {
	var out bytes.Buffer
	opts := Opts{Stdout: &out, DryRun: true}
	if err := sudoTeeWrite(context.Background(), opts, filepath.Join(t.TempDir(), "f"), "hello\n"); err != nil {
		t.Fatalf("sudoTeeWrite dry-run: %v", err)
	}
	if err := sudoTeeAppend(context.Background(), opts, filepath.Join(t.TempDir(), "a"), "line"); err != nil {
		t.Fatalf("sudoTeeAppend dry-run: %v", err)
	}
	if !strings.Contains(out.String(), "[dry-run]") {
		t.Errorf("expected dry-run output, got %q", out.String())
	}
	_ = os.ErrNotExist
}
