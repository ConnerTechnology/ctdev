package backup

import "testing"

func TestResticVersionCheck(t *testing.T) {
	for out, ok := range map[string]bool{
		"restic 0.19.1 compiled with go1.26.4 on linux/amd64\n": true,
		"restic 0.17.0 compiled with go1.22.5 on darwin/arm64":  true,
		"restic 1.0.0 compiled with go1.30 on linux/amd64":      true,
		"restic 0.16.4 compiled with go1.22.2 on linux/amd64":   false, // Ubuntu 24.04's apt
		"restic 0.12.1 compiled with go1.18.1 on linux/amd64":   false, // Ubuntu 22.04's apt
		"something else": false,
	} {
		if err := resticVersionOK(out); (err == nil) != ok {
			t.Errorf("%q: err = %v, want ok %v", out, err, ok)
		}
	}
}
