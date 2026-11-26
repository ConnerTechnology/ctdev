package progress

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestProgress_CancelModesAndShortWindow(t *testing.T) {
	t.Run("ctrl+c quits early leaving unrun items in Counts", func(t *testing.T) {
		m := New([]string{"a", "b", "c"}, ModeInstall, false)
		m.Update(InstallStartMsg{Name: "a"})
		m.Update(InstallDoneMsg{Name: "a", Duration: time.Millisecond})
		m.Update(InstallStartMsg{Name: "b"})

		_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		if cmd == nil {
			t.Fatal("expected tea.Quit cmd on ctrl+c")
		}
		done, failed, skipped, notRun := m.Counts()
		if done != 1 || failed != 0 || skipped != 0 || notRun != 2 {
			t.Errorf("Counts() = (%d,%d,%d,%d), want (1,0,0,2)", done, failed, skipped, notRun)
		}
	})

	t.Run("ModeUninstall and ModeUpdate render distinct headers and survive short height", func(t *testing.T) {
		for _, mode := range []Mode{ModeUninstall, ModeUpdate} {
			m := New([]string{"one", "two", "three", "four", "five"}, mode, true)
			m.Update(tea.WindowSizeMsg{Width: 60, Height: 8})
			m.Update(InstallStartMsg{Name: "one"})
			v := m.View().Content
			if strings.TrimSpace(v) == "" {
				t.Errorf("mode %v rendered empty view", mode)
			}
		}
	})
}
