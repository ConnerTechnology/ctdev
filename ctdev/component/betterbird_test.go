package component

import (
	"strings"
	"testing"
)

func TestBetterbirdMajor(t *testing.T) {
	cases := []struct {
		file, want string
		wantErr    bool
	}{
		{file: "betterbird-153.4.0esr-bb10.en-US.linux-x86_64.tar.xz", want: "153"},
		{file: "betterbird-140.13.0esr-bb25.en-US.linux-x86_64.tar.xz", want: "140"},
		{file: "thunderbird-153.4.0esr.en-US.linux-x86_64.tar.xz", wantErr: true},
		{file: "betterbird-esr.en-US.linux-x86_64.tar.xz", wantErr: true},
		{file: "betterbird-", wantErr: true},
	}
	for _, c := range cases {
		got, err := betterbirdMajor(c.file)
		if (err != nil) != c.wantErr {
			t.Errorf("betterbirdMajor(%q) err = %v, wantErr %v", c.file, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("betterbirdMajor(%q) = %q, want %q", c.file, got, c.want)
		}
	}
}

func TestBetterbirdDesktopEntryPointsAtInstallDir(t *testing.T) {
	// The tarball has no `betterbird` on PATH, so the launcher must use the
	// absolute path or the menu entry does nothing.
	data, err := Configs.ReadFile("configs/betterbird/eu.betterbird.Betterbird.desktop")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Exec=" + betterbirdInstallDir + "/betterbird %u",
		"Icon=" + betterbirdInstallDir + "/chrome/icons/default/default256.png",
	} {
		if !strings.Contains(string(data), "\n"+want+"\n") {
			t.Errorf("desktop entry missing line %q", want)
		}
	}
}
