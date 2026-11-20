package cmd

import "testing"

func TestEntireUpdateItems(t *testing.T) {
	tests := []struct {
		name, current, latest string
		want                  bool
	}{
		{"behind", "0.13.3", "0.13.4", true},
		{"current", "0.13.4", "0.13.4", false},
		{"ahead of latest", "0.14.0", "0.13.4", false},
		{"not installed", "", "0.13.4", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := entireUpdateItems(tt.current, tt.latest)
			if (len(items) == 1) != tt.want || len(items) > 1 {
				t.Fatalf("got %+v, want an item: %v", items, tt.want)
			}
			if tt.want {
				it := items[0]
				if it.Name != "entire" || it.Source != "cli" || it.CurrentVer != tt.current || it.NewVer != tt.latest {
					t.Errorf("item = %+v", it)
				}
				if _, ok := cliUpdateStep(it); !ok {
					t.Error("the entire item has no update step")
				}
			}
		})
	}
}

// On macOS the cask is upgraded with the other managed casks.
func TestEntireCaskIsManaged(t *testing.T) {
	if managedCasks["entire"] != "entire" {
		t.Error(`managedCasks must map the "entire" cask to the entire component`)
	}
}
