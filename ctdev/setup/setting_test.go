package setup

import "testing"

func TestNeedsApply(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		force    bool
		current  string
		desired  string
		expected bool
	}{
		{"disabled ignores different values", false, false, "a", "b", false},
		{"disabled ignores force", false, true, "a", "b", false},
		{"enabled force always true", true, true, "same", "same", true},
		{"enabled same values no force", true, false, "same", "same", false},
		{"enabled different values", true, false, "old", "new", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss := SettingState{
				Enabled:      tt.enabled,
				CurrentValue: tt.current,
				DesiredValue: tt.desired,
			}
			if got := ss.NeedsApply(tt.force); got != tt.expected {
				t.Errorf("NeedsApply(%v) = %v, want %v", tt.force, got, tt.expected)
			}
		})
	}
}

func TestFilterByHardware(t *testing.T) {
	t.Run("nil HardwareFn passes through", func(t *testing.T) {
		settings := []Setting{{Name: "a", HardwareFn: nil}}
		got := FilterByHardware(settings)
		if len(got) != 1 || got[0].Name != "a" {
			t.Errorf("expected [a], got %v", got)
		}
	})

	t.Run("false HardwareFn excludes", func(t *testing.T) {
		settings := []Setting{{Name: "a", HardwareFn: func() bool { return false }}}
		got := FilterByHardware(settings)
		if len(got) != 0 {
			t.Errorf("expected empty, got %v", got)
		}
	})

	t.Run("true HardwareFn includes", func(t *testing.T) {
		settings := []Setting{{Name: "a", HardwareFn: func() bool { return true }}}
		got := FilterByHardware(settings)
		if len(got) != 1 {
			t.Errorf("expected 1 result, got %d", len(got))
		}
	})

	t.Run("mix of nil and non-nil", func(t *testing.T) {
		settings := []Setting{
			{Name: "a", HardwareFn: nil},
			{Name: "b", HardwareFn: func() bool { return false }},
			{Name: "c", HardwareFn: func() bool { return true }},
		}
		got := FilterByHardware(settings)
		if len(got) != 2 {
			t.Fatalf("expected 2 results, got %d", len(got))
		}
		if got[0].Name != "a" || got[1].Name != "c" {
			t.Errorf("expected [a c], got [%s %s]", got[0].Name, got[1].Name)
		}
	})
}

func TestInitStates(t *testing.T) {
	t.Run("nil DetectFunc gives empty CurrentValue", func(t *testing.T) {
		settings := []Setting{{Name: "a", Default: "def", DetectFunc: nil}}
		states := InitStates(settings)
		if states[0].CurrentValue != "" {
			t.Errorf("expected empty, got %q", states[0].CurrentValue)
		}
	})

	t.Run("DetectFunc sets CurrentValue", func(t *testing.T) {
		settings := []Setting{{Name: "a", Default: "def", DetectFunc: func() string { return "foo" }}}
		states := InitStates(settings)
		if states[0].CurrentValue != "foo" {
			t.Errorf("expected foo, got %q", states[0].CurrentValue)
		}
	})

	t.Run("DesiredValue set to Default", func(t *testing.T) {
		settings := []Setting{{Name: "a", Default: "mydefault"}}
		states := InitStates(settings)
		if states[0].DesiredValue != "mydefault" {
			t.Errorf("expected mydefault, got %q", states[0].DesiredValue)
		}
	})

	t.Run("Enabled is true", func(t *testing.T) {
		settings := []Setting{{Name: "a"}}
		states := InitStates(settings)
		if !states[0].Enabled {
			t.Error("expected Enabled=true")
		}
	})
}

func TestRegistryHasPeripheralsCategory(t *testing.T) {
	var found []string
	for _, s := range Registry {
		if s.Category == "Peripherals & KVM" {
			found = append(found, s.Name)
		}
	}
	if len(found) == 0 {
		t.Fatal("expected at least one setting in 'Peripherals & KVM' category")
	}

	expected := map[string]bool{
		"Logitech KVM mouse fix": false,
		"Hide drives":            false,
	}
	for _, name := range found {
		if _, ok := expected[name]; ok {
			expected[name] = true
		}
	}
	for name, seen := range expected {
		if !seen {
			t.Errorf("expected %q in Peripherals & KVM category", name)
		}
	}
}

func TestLogitechKVMFixHasHardwareFn(t *testing.T) {
	for _, s := range Registry {
		if s.Name == "Logitech KVM mouse fix" {
			if s.HardwareFn == nil {
				t.Error("Logitech KVM mouse fix should have HardwareFn set")
			}
			if s.DetectFunc == nil {
				t.Error("Logitech KVM mouse fix should have DetectFunc set")
			}
			if s.ApplyFunc == nil {
				t.Error("Logitech KVM mouse fix should have ApplyFunc set")
			}
			return
		}
	}
	t.Fatal("Logitech KVM mouse fix not found in Registry")
}

func TestHideDrivesHasNoHardwareFn(t *testing.T) {
	for _, s := range Registry {
		if s.Name == "Hide drives" {
			if s.HardwareFn != nil {
				t.Error("Hide drives should not have HardwareFn (always visible)")
			}
			if s.DetectFunc == nil {
				t.Error("Hide drives should have DetectFunc set")
			}
			if s.ApplyFunc == nil {
				t.Error("Hide drives should have ApplyFunc set")
			}
			return
		}
	}
	t.Fatal("Hide drives not found in Registry")
}

func TestCategories(t *testing.T) {
	t.Run("deduplication", func(t *testing.T) {
		settings := []Setting{
			{Category: "cat1"},
			{Category: "cat1"},
		}
		cats := Categories(settings)
		if len(cats) != 1 {
			t.Errorf("expected 1 category, got %d", len(cats))
		}
	})

	t.Run("order preservation", func(t *testing.T) {
		settings := []Setting{
			{Category: "beta"},
			{Category: "alpha"},
			{Category: "beta"},
			{Category: "gamma"},
		}
		cats := Categories(settings)
		expected := []string{"beta", "alpha", "gamma"}
		if len(cats) != len(expected) {
			t.Fatalf("expected %d categories, got %d", len(expected), len(cats))
		}
		for i, c := range cats {
			if c != expected[i] {
				t.Errorf("index %d: expected %q, got %q", i, expected[i], c)
			}
		}
	})

	t.Run("empty input", func(t *testing.T) {
		cats := Categories(nil)
		if len(cats) != 0 {
			t.Errorf("expected empty, got %v", cats)
		}
	})
}
