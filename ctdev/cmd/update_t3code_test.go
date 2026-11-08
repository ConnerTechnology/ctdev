package cmd

import "testing"

func TestT3CodeUpdateItems(t *testing.T) {
	tests := []struct {
		name, cli, desktop, latest string
		want                       []string
	}{
		{"both behind", "0.1.6", "0.1.6", "0.1.7", []string{"t3code", "t3code-desktop"}},
		{"cli current, desktop behind", "0.1.7", "0.1.5", "0.1.7", []string{"t3code-desktop"}},
		{"only the cli installed", "0.1.6", "", "0.1.7", []string{"t3code"}},
		{"both current", "0.1.7", "0.1.7", "0.1.7", nil},
		{"ahead of latest", "0.2.0", "", "0.1.7", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := t3codeUpdateItems(tt.cli, tt.desktop, tt.latest)
			if len(items) != len(tt.want) {
				t.Fatalf("got %d items %+v, want %v", len(items), items, tt.want)
			}
			for i, item := range items {
				if item.Name != tt.want[i] || item.Source != "cli" || item.NewVer != tt.latest {
					t.Errorf("item %d = %+v, want %s from cli at %s", i, item, tt.want[i], tt.latest)
				}
			}
		})
	}
}
