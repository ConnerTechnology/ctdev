package sysutil

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadFile_Success(t *testing.T) {
	body := []byte("payload-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	if err := DownloadFile(context.Background(), srv.URL, dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("dest body = %q, want %q", got, body)
	}
}

func TestDownloadFile_HTTPErrorNoFileCreated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	err := DownloadFile(context.Background(), srv.URL, dest)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Errorf("expected dest to be absent after HTTP error; stat err = %v", statErr)
	}
}

func TestDownloadFile_PartialTransferCleansUp(t *testing.T) {
	// Advertise a longer Content-Length than we send so io.Copy returns
	// ErrUnexpectedEOF. DownloadFile must remove the partial file.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "short")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			return
		}
		conn.Close()
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	err := DownloadFile(context.Background(), srv.URL, dest)
	if err == nil {
		t.Fatal("expected error from truncated response")
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Errorf("expected dest to be removed on partial transfer; stat err = %v", statErr)
	}
}

func TestVerifyChecksum(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.bin")
	content := []byte("hello world")
	if err := os.WriteFile(file, content, 0644); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(content)
	expected := hex.EncodeToString(h[:])

	if err := VerifyChecksum(file, expected); err != nil {
		t.Errorf("expected valid checksum, got: %v", err)
	}
	if err := VerifyChecksum(file, "0000badchecksum"); err == nil {
		t.Error("expected checksum mismatch error")
	}
}

func TestVerifyChecksumFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.bin")
	content := []byte("test data")
	if err := os.WriteFile(file, content, 0644); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(content)
	hash := hex.EncodeToString(h[:])

	checksums := filepath.Join(dir, "SHA256SUMS")
	csContent := hash + "  test.bin\ndeadbeef  other.bin\n"
	if err := os.WriteFile(checksums, []byte(csContent), 0644); err != nil {
		t.Fatal(err)
	}

	if err := VerifyChecksumFile(file, checksums); err != nil {
		t.Errorf("expected valid checksum file match, got: %v", err)
	}
}

func TestVerifyChecksumFileFormats(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.bin")
	content := []byte("test data")
	if err := os.WriteFile(file, content, 0644); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(content)
	hash := hex.EncodeToString(h[:])

	cases := map[string]string{
		"binary marker":   hash + " *test.bin\n",
		"uppercase hash":  strings.ToUpper(hash) + "  test.bin\n",
		"trailing column": hash + "  test.bin  extra\n",
	}
	for name, csContent := range cases {
		t.Run(name, func(t *testing.T) {
			checksums := filepath.Join(dir, "SHA256SUMS")
			if err := os.WriteFile(checksums, []byte(csContent), 0644); err != nil {
				t.Fatal(err)
			}
			if err := VerifyChecksumFile(file, checksums); err != nil {
				t.Errorf("expected match for %s format, got: %v", name, err)
			}
		})
	}
}

func TestVerifyChecksumFileMissing(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.bin")
	if err := os.WriteFile(file, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	checksums := filepath.Join(dir, "SHA256SUMS")
	if err := os.WriteFile(checksums, []byte("abc123  other.bin\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := VerifyChecksumFile(file, checksums)
	if err == nil {
		t.Error("expected error for missing file in checksums")
	}
}

func writeTestArchive(t *testing.T, content string) (path, sum string) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "asset.bin")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256([]byte(content))
	return path, hex.EncodeToString(h[:])
}

func TestVerifyDownload_ChecksumFuncMatch(t *testing.T) {
	path, sum := writeTestArchive(t, "binary")
	var gotVer, gotOS, gotArch string
	spec := GitHubBinarySpec{Checksum: func(_ context.Context, ver, goos, goarch string) (string, error) {
		gotVer, gotOS, gotArch = ver, goos, goarch
		return strings.ToUpper(sum), nil // manifests may publish uppercase hex
	}}
	if err := verifyDownload(context.Background(), spec, "1.2.3", "linux", "arm64", path, t.TempDir()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotVer != "1.2.3" || gotOS != "linux" || gotArch != "arm64" {
		t.Errorf("Checksum called with (%q, %q, %q)", gotVer, gotOS, gotArch)
	}
}

func TestVerifyDownload_ChecksumFuncMismatch(t *testing.T) {
	path, _ := writeTestArchive(t, "binary")
	spec := GitHubBinarySpec{Checksum: func(context.Context, string, string, string) (string, error) {
		return strings.Repeat("0", 64), nil
	}}
	err := verifyDownload(context.Background(), spec, "1.2.3", "linux", "amd64", path, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func TestVerifyDownload_ChecksumFuncError(t *testing.T) {
	path, _ := writeTestArchive(t, "binary")
	spec := GitHubBinarySpec{Checksum: func(context.Context, string, string, string) (string, error) {
		return "", fmt.Errorf("no checksum for 1.2.3")
	}}
	err := verifyDownload(context.Background(), spec, "1.2.3", "linux", "amd64", path, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "no checksum for 1.2.3") {
		t.Fatalf("a lookup failure must fail the install, got %v", err)
	}
}

func TestVerifyDownload_NoChecksumConfigured(t *testing.T) {
	path, _ := writeTestArchive(t, "binary")
	if err := verifyDownload(context.Background(), GitHubBinarySpec{}, "1.2.3", "linux", "amd64", path, t.TempDir()); err != nil {
		t.Fatalf("specs without a checksum behave as before, got %v", err)
	}
}

func TestVerifyDownload_ChecksumURLStillWorks(t *testing.T) {
	path, sum := writeTestArchive(t, "binary")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", sum, filepath.Base(path))
	}))
	defer srv.Close()
	spec := GitHubBinarySpec{ChecksumURL: func(string, string, string) string { return srv.URL }}
	if err := verifyDownload(context.Background(), spec, "1.2.3", "linux", "amd64", path, t.TempDir()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A pinned bz2 release installs into a user-owned directory without sudo, and
// only after its checksum matches.
func TestDownloadGitHubBinary_PinnedBz2UserInstall(t *testing.T) {
	// bzip2 of "fake restic binary\n"; the stdlib has no bzip2 writer.
	archive, err := hex.DecodeString("425a6839314159265359d8ec154c000004d180001040003b291c202000228d0f49b53c8400007d12451fd3484c17eb177245385090d8ec154c")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archive)
	for _, tc := range []struct {
		name    string
		sums    string
		wantErr bool
	}{
		{"checksum matches", hex.EncodeToString(sum[:]) + "  tool_1.2.3.bz2\n", false},
		{"checksum mismatch", strings.Repeat("0", 64) + "  tool_1.2.3.bz2\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1.2.3/tool_1.2.3.bz2":
					w.Write(archive)
				case "/v1.2.3/SHA256SUMS":
					fmt.Fprint(w, tc.sums)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			dest := filepath.Join(t.TempDir(), "bin", "tool")
			ver, err := DownloadGitHubBinary(context.Background(), Opts{Stdout: os.Stdout}, GitHubBinarySpec{
				Repo:    "example/tool",
				Version: "1.2.3",
				ArchiveURL: func(ver, goos, goarch string) string {
					return fmt.Sprintf("%s/v%s/tool_%s.bz2", srv.URL, ver, ver)
				},
				ChecksumURL: func(ver, goos, goarch string) string {
					return fmt.Sprintf("%s/v%s/SHA256SUMS", srv.URL, ver)
				},
				InstallDest: dest,
				ArchFormat:  "bz2",
				UserInstall: true,
			})
			if tc.wantErr {
				if err == nil {
					t.Fatal("want a checksum error, got none")
				}
				if _, statErr := os.Stat(dest); statErr == nil {
					t.Error("binary installed despite a checksum mismatch")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if ver != "1.2.3" {
				t.Errorf("version = %q, want the pin", ver)
			}
			got, err := os.ReadFile(dest)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "fake restic binary\n" {
				t.Errorf("installed contents = %q", got)
			}
			if fi, err := os.Stat(dest); err != nil || fi.Mode().Perm()&0o100 == 0 {
				t.Errorf("installed binary is not executable: %v", err)
			}
		})
	}
}

// A release whose hash comes from a Checksum hook installs the raw binary only
// after the hash matches, and installs nothing when it doesn't or can't be found.
func TestDownloadGitHubBinary_ChecksumHookGatesInstall(t *testing.T) {
	body := []byte("fake herdr binary\n")
	sum := sha256.Sum256(body)
	for _, tc := range []struct {
		name    string
		hash    string
		hashErr error
		wantErr string
	}{
		{"hash matches", hex.EncodeToString(sum[:]), nil, ""},
		{"hash mismatch", strings.Repeat("0", 64), nil, "checksum mismatch"},
		{"hash not published", "", fmt.Errorf("manifest has no checksum for v1.2.3"), "no checksum for v1.2.3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write(body)
			}))
			defer srv.Close()

			dest := filepath.Join(t.TempDir(), "bin", "tool")
			_, err := DownloadGitHubBinary(context.Background(), Opts{Stdout: os.Stdout}, GitHubBinarySpec{
				Repo:    "example/tool",
				Version: "1.2.3",
				ArchiveURL: func(ver, goos, goarch string) string {
					return fmt.Sprintf("%s/v%s/tool-%s-%s", srv.URL, ver, goos, goarch)
				},
				Checksum: func(context.Context, string, string, string) (string, error) {
					return tc.hash, tc.hashErr
				},
				InstallDest: dest,
				UserInstall: true,
			})
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want an error containing %q, got %v", tc.wantErr, err)
				}
				if _, statErr := os.Stat(dest); statErr == nil {
					t.Error("binary installed despite a failed checksum")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(dest)
			if err != nil || string(got) != string(body) {
				t.Errorf("installed contents = %q, %v", got, err)
			}
		})
	}
}
