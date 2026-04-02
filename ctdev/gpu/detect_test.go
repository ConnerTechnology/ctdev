package gpu

import (
	"os"
	"testing"
)

func TestParseSecureBootState(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect bool
	}{
		{"enabled", "SecureBoot enabled\n", true},
		{"disabled", "SecureBoot disabled\n", false},
		{"empty", "", false},
		{"with extra text", "EFI variables are not supported on this system\n", false},
		{"enabled with prefix", "SecureBoot enabled in shim\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSecureBootState(tt.input)
			if got != tt.expect {
				t.Errorf("parseSecureBootState(%q) = %v, want %v", tt.input, got, tt.expect)
			}
		})
	}
}

func TestParseLsmodNvidia(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect bool
	}{
		{
			"nvidia loaded",
			"Module                  Size  Used by\nnvidia              62914560  47\nnvidia_uvm            6189056  0\n",
			true,
		},
		{
			"nvidia not loaded",
			"Module                  Size  Used by\ni915                 3145728  10\n",
			false,
		},
		{"empty", "", false},
		{
			"nvidia_uvm but not nvidia base",
			"Module                  Size  Used by\nnvidia_uvm            6189056  0\n",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLsmodNvidia(tt.input)
			if got != tt.expect {
				t.Errorf("parseLsmodNvidia(%q) = %v, want %v", tt.input, got, tt.expect)
			}
		})
	}
}

func TestParseNvidiaSmiVersion(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"version found", "550.120\n", "550.120"},
		{"smi failed", "NVIDIA-SMI has failed because...\n", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNvidiaSmiVersion(tt.input)
			if got != tt.expect {
				t.Errorf("parseNvidiaSmiVersion(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestParseModinfoVersion(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			"version found",
			"filename:       /lib/modules/6.8.0/updates/dkms/nvidia.ko\nversion:        550.120\n",
			"550.120",
		},
		{
			"not found",
			"filename:       /lib/modules/6.8.0/updates/dkms/nvidia.ko\nlicense:        NVIDIA\n",
			"",
		},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseModinfoVersion(tt.input)
			if got != tt.expect {
				t.Errorf("parseModinfoVersion(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestParseModinfoVariant(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			"open dual mit/gpl",
			"license:        Dual MIT/GPL\n",
			"open",
		},
		{
			"open gpl",
			"license:        GPL\n",
			"open",
		},
		{
			"closed proprietary",
			"license:        NVIDIA\n",
			"closed",
		},
		{
			"unknown empty",
			"filename:       /lib/modules/nvidia.ko\n",
			"unknown",
		},
		{"empty input", "", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseModinfoVariant(tt.input)
			if got != tt.expect {
				t.Errorf("parseModinfoVariant(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestParseGlxinfoRenderer(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			"nvidia",
			"OpenGL renderer string: NVIDIA GeForce RTX 4090/PCIe/SSE2\n",
			"nvidia",
		},
		{
			"llvmpipe",
			"OpenGL renderer string: llvmpipe (LLVM 15.0.7, 256 bits)\n",
			"llvmpipe",
		},
		{
			"other",
			"OpenGL renderer string: Mesa Intel(R) UHD Graphics 630\n",
			"unknown",
		},
		{"no match", "something else\n", "unknown"},
		{"empty", "", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGlxinfoRenderer(tt.input)
			if got != tt.expect {
				t.Errorf("parseGlxinfoRenderer(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestParseOpenSSLFingerprint(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			"sha1 fingerprint",
			"SHA1 Fingerprint=AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD\n",
			"AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD",
		},
		{"empty", "", ""},
		{"no equals", "no fingerprint here", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseOpenSSLFingerprint(tt.input)
			if got != tt.expect {
				t.Errorf("parseOpenSSLFingerprint(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestParseDKMSFrameworkConf(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect bool
	}{
		{
			"configured",
			"# DKMS framework config\nmok_signing_key=" + MOKPriv + "\nmok_certificate=" + MOKCert + "\n",
			true,
		},
		{
			"commented out",
			"# mok_signing_key=" + MOKPriv + "\n# mok_certificate=" + MOKCert + "\n",
			false,
		},
		{
			"missing cert",
			"mok_signing_key=" + MOKPriv + "\n",
			false,
		},
		{
			"wrong paths",
			"mok_signing_key=/wrong/path\nmok_certificate=/wrong/cert\n",
			false,
		},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDKMSFrameworkConf(tt.input)
			if got != tt.expect {
				t.Errorf("parseDKMSFrameworkConf(%q) = %v, want %v", tt.input, got, tt.expect)
			}
		})
	}
}

func TestParseModinfoSigner(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			"signer found",
			"filename:       /lib/modules/nvidia.ko\nsigner:         Custom NVIDIA Module Signing\nsig_id:         PKCS#7\n",
			"Custom NVIDIA Module Signing",
		},
		{
			"no signer",
			"filename:       /lib/modules/nvidia.ko\nversion:        550.120\n",
			"",
		},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseModinfoSigner(tt.input)
			if got != tt.expect {
				t.Errorf("parseModinfoSigner(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestParseDKMSStatus(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			"nvidia found",
			"nvidia/550.120, 6.8.0-51-generic, x86_64: installed\n",
			"nvidia/550.120",
		},
		{
			"no nvidia",
			"virtualbox/7.0.14, 6.8.0-51-generic, x86_64: installed\n",
			"",
		},
		{"empty", "", ""},
		{
			"multiple modules",
			"virtualbox/7.0.14, 6.8.0-51-generic, x86_64: installed\nnvidia/550.120, 6.8.0-51-generic, x86_64: installed\n",
			"nvidia/550.120",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDKMSStatus(tt.input)
			if got != tt.expect {
				t.Errorf("parseDKMSStatus(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestParseOpenSSLSubjectCN(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			"standard format",
			"subject=CN = Custom NVIDIA Module Signing\n",
			"Custom NVIDIA Module Signing",
		},
		{
			"compact format",
			"subject=CN=Custom NVIDIA Module Signing\n",
			"Custom NVIDIA Module Signing",
		},
		{"empty", "", ""},
		{"no CN", "subject=O = Foo\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseOpenSSLSubjectCN(tt.input)
			if got != tt.expect {
				t.Errorf("parseOpenSSLSubjectCN(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestFindNvidiaModulesInDir(t *testing.T) {
	dir := t.TempDir()

	// Create test files
	files := []string{
		"nvidia.ko",
		"nvidia-uvm.ko.zst",
		"nvidia-drm.ko.xz",
		"i915.ko",
		"readme.txt",
	}
	for _, f := range files {
		if err := writeFile(dir+"/"+f, ""); err != nil {
			t.Fatal(err)
		}
	}

	got := findNvidiaModulesInDir(dir)
	if len(got) != 3 {
		t.Errorf("expected 3 nvidia modules, got %d: %v", len(got), got)
	}
}

func TestFindNvidiaModulesInDir_NonExistent(t *testing.T) {
	got := findNvidiaModulesInDir("/nonexistent/path")
	if got != nil {
		t.Errorf("expected nil for nonexistent dir, got %v", got)
	}
}

func writeFile(path, content string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}
