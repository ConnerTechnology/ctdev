package cmd

import (
	"bufio"
	"context"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/setup"
)

func multiPickSetting() *setup.Setting {
	return &setup.Setting{
		Name:    "Hide drives",
		Control: setup.ControlMultiPick,
		ChoicesFunc: func(context.Context) []setup.PickerChoice {
			return []setup.PickerChoice{
				{Value: "AAAA-1111", Description: "/dev/sda1 · vfat"},
				{Value: "BBBB-2222", Description: "/dev/sdb1 · ntfs"},
				{Value: "CCCC-3333", Description: "/dev/sdb2 · ntfs"},
			}
		},
	}
}

func TestPromptMultiPick(t *testing.T) {
	orig := stdinScanner
	t.Cleanup(func() { stdinScanner = orig })

	tests := []struct {
		name    string
		current string
		input   string
		want    string
	}{
		{"enter keeps current", "BBBB-2222", "", "BBBB-2222"},
		{"numbers pick exactly those", "BBBB-2222", "3 1", "AAAA-1111,CCCC-3333"},
		{"commas work too", setup.PicksNone, "2,3", "BBBB-2222,CCCC-3333"},
		{"0 checks nothing", "AAAA-1111,BBBB-2222", "0", setup.PicksNone},
		{"none checks nothing", "AAAA-1111", "none", setup.PicksNone},
		{"out of range keeps current", "AAAA-1111", "4", "AAAA-1111"},
		{"junk keeps current", "AAAA-1111", "x", "AAAA-1111"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdinScanner = bufio.NewScanner(strings.NewReader(tt.input + "\n"))
			st := &setup.SettingState{Setting: multiPickSetting(), CurrentValue: tt.current}
			var got string
			out := captureStdout(t, func() {
				var err error
				got, err = promptMultiPick(context.Background(), st)
				if err != nil {
					t.Errorf("promptMultiPick: %v", err)
				}
			})
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			if !strings.Contains(out, "/dev/sdb2 · ntfs") {
				t.Errorf("choices not listed: %q", out)
			}
		})
	}
}

func TestPromptMultiPick_MarksChecked(t *testing.T) {
	orig := stdinScanner
	t.Cleanup(func() { stdinScanner = orig })
	stdinScanner = bufio.NewScanner(strings.NewReader("\n"))
	st := &setup.SettingState{Setting: multiPickSetting(), CurrentValue: "BBBB-2222"}
	out := captureStdout(t, func() { _, _ = promptMultiPick(context.Background(), st) })
	if !strings.Contains(out, "[x] 2)") || !strings.Contains(out, "[ ] 1)") {
		t.Errorf("checked partition not marked: %q", out)
	}
}

func TestPromptMultiPick_NoChoices(t *testing.T) {
	s := &setup.Setting{Name: "Hide drives", Control: setup.ControlMultiPick,
		ChoicesFunc: func(context.Context) []setup.PickerChoice { return nil }}
	st := &setup.SettingState{Setting: s, CurrentValue: setup.PicksNone}
	var got string
	captureStdout(t, func() { got, _ = promptMultiPick(context.Background(), st) })
	if got != setup.PicksNone {
		t.Errorf("got %q, want current kept", got)
	}
}

func TestShowSetting_NoRecommendationNoMarker(t *testing.T) {
	st := &setup.SettingState{Setting: multiPickSetting(), CurrentValue: "AAAA-1111"}
	out := captureStdout(t, func() { showSetting(st) })
	if strings.Contains(out, "recommended") {
		t.Errorf("a setting with no recommendation shows one: %q", out)
	}
}
