package backupapp

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPickerWindow_ShortTerminalNeverPanics(t *testing.T) {
	for _, n := range []int{0, 1, 2, 3, 10} {
		items := make([]Item, n)
		for i := range items {
			items[i] = Item{Cells: []string{"item"}}
		}
		pk := newPicker(Picker{Items: items})
		for _, h := range []int{0, 5, 10, 16, 17, 18, 25} {
			first, last := pk.window(h)
			if first < 0 || last < first || last > n {
				t.Errorf("n=%d h=%d: window = (%d, %d) out of bounds [0,%d]", n, h, first, last, n)
			}
			// Rendering at this narrow/short terminal size must never panic.
			_ = pk.view(40, h)
		}
	}
}

func TestPlace_AllBackends(t *testing.T) {
	t.Run("b2 default and custom path", func(t *testing.T) {
		addr, creds, at := place(Values{"store": "b2", "b2.bucket": "example-bucket", "B2_ACCOUNT_ID": "id", "B2_ACCOUNT_KEY": "key"})
		if addr != "b2:example-bucket:ctdev" || creds["B2_ACCOUNT_ID"] != "id" || creds["B2_ACCOUNT_KEY"] != "key" || at != "b2.bucket" {
			t.Errorf("b2 default = %q %v %q", addr, creds, at)
		}
		addr, _, _ = place(Values{"store": "b2", "b2.bucket": "example-bucket", "b2.path": "custom"})
		if addr != "b2:example-bucket:custom" {
			t.Errorf("b2 custom = %q", addr)
		}
	})

	t.Run("s3", func(t *testing.T) {
		addr, creds, at := place(Values{
			"store": "s3", "s3.endpoint": "s3.example.invalid", "s3.bucket": "bkt",
			"AWS_ACCESS_KEY_ID": "ak", "AWS_SECRET_ACCESS_KEY": "sk",
		})
		if addr != "s3:https://s3.example.invalid/bkt/ctdev" || creds["AWS_ACCESS_KEY_ID"] != "ak" || at != "s3.bucket" {
			t.Errorf("s3 = %q %v %q", addr, creds, at)
		}
	})

	t.Run("sftp", func(t *testing.T) {
		addr, creds, at := place(Values{"store": "sftp", "sftp.host": "nas.example.invalid", "sftp.path": "/srv/backups"})
		if addr != "sftp:nas.example.invalid:/srv/backups" || len(creds) != 0 || at != "sftp.path" {
			t.Errorf("sftp = %q %v %q", addr, creds, at)
		}
	})

	t.Run("local", func(t *testing.T) {
		addr, _, at := place(Values{"store": "local", "local.path": "/tmp/backups"})
		if addr != "/tmp/backups" || at != "local.path" {
			t.Errorf("local = %q %q", addr, at)
		}
	})
}

func TestModel_WindowSizeNarrowAndShort(t *testing.T) {
	m := &model{}
	m.Update(tea.WindowSizeMsg{Width: 32, Height: 10})
	if m.width != 32 || m.height != 10 {
		t.Errorf("window size = %dx%d, want 32x10", m.width, m.height)
	}
	// Render with picker active at short height.
	m.page = Page{Title: "Backups"}
	m.step = newPicker(Picker{
		Page:  Page{Title: "Backups"},
		Items: []Item{{Cells: []string{"one"}}, {Cells: []string{"two"}}},
	})
	v := m.View()
	if v.Content == "" {
		t.Error("expected non-empty rendered view")
	}
}
