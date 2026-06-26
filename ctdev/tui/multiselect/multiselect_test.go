package multiselect

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestViewClipsRowsToWidth(t *testing.T) {
	groups := []Group{
		{Key: "g", Title: "Group", Items: []Item{
			{ID: "1", Primary: "short", Selectable: true, Bulk: true},
			{ID: "2", Primary: "verylongcomponentname", Secondary: strings.Repeat("detail ", 20), Selectable: true, Bulk: true},
		}},
	}
	m := New(groups, Options{Title: "T"})
	const width = 30
	m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
	for _, line := range strings.Split(m.View().Content, "\n") {
		if w := lipgloss.Width(line); w > width {
			t.Errorf("rendered line exceeds width %d (got %d): %q", width, w, line)
		}
	}
}

func sample() []Group {
	return []Group{
		{Key: "apt", Title: "System Packages (apt)", Items: []Item{
			{ID: "a1", Primary: "curl", Secondary: "7.88 → 8.0", Selectable: true, Bulk: true},
			{ID: "a2", Primary: "git", Secondary: "2.39 → 2.40", Selectable: true, Bulk: true},
		}},
		{Key: "infra", Title: "Infra", Items: []Item{
			{ID: "i1", Primary: "docker", Selectable: true, Bulk: false, Marked: true}, // installed-like
			{ID: "i2", Primary: "nginx", Selectable: false, Note: "(linux only)"},      // unsupported
			{ID: "i3", Primary: "redis", Selectable: true, Bulk: true},
		}},
	}
}

func TestPreselectAllOnlyBulk(t *testing.T) {
	m := New(sample(), Options{PreselectAll: true})
	// a1, a2, i3 are Bulk; i1 is not Bulk; i2 is not selectable.
	if got := len(m.selected); got != 3 {
		t.Fatalf("preselect: got %d selected, want 3 (%v)", got, m.selected)
	}
	for _, id := range []string{"a1", "a2", "i3"} {
		if !m.selected[id] {
			t.Errorf("expected %q preselected", id)
		}
	}
	if m.selected["i1"] || m.selected["i2"] {
		t.Errorf("non-bulk/unselectable items must not be preselected: %v", m.selected)
	}
}

func TestPreselectAllSkipsNoPreselect(t *testing.T) {
	groups := []Group{
		{Key: "apt", Title: "System Packages (apt)", Items: []Item{
			{ID: "a1", Primary: "curl", Selectable: true, Bulk: true, NoPreselect: true}, // risky default
			{ID: "a2", Primary: "git", Selectable: true, Bulk: true},
		}},
	}
	m := New(groups, Options{PreselectAll: true})
	if m.selected["a1"] {
		t.Error("NoPreselect item must not be pre-selected")
	}
	if !m.selected["a2"] {
		t.Error("ordinary bulk item should still be pre-selected")
	}
	// It is still selectable on demand.
	m.moveCursor(1) // header -> a1
	m.toggle()
	if !m.selected["a1"] {
		t.Error("NoPreselect item should still be toggleable on")
	}
}

func TestCursorStartsLandableAndSkipsUnsupported(t *testing.T) {
	m := New(sample(), Options{})
	if !m.landable(m.cursor) {
		t.Fatalf("initial cursor %d not landable", m.cursor)
	}
	// Walk to the end; cursor must never rest on the unsupported row (i2 at idx 5).
	for i := 0; i < 20; i++ {
		m.moveCursor(1)
		if m.rows[m.cursor].header {
			continue
		}
		if !m.rows[m.cursor].item.Selectable {
			t.Fatalf("cursor landed on non-selectable row %d (%s)", m.cursor, m.rows[m.cursor].item.Primary)
		}
	}
}

func TestToggleItemAndGroup(t *testing.T) {
	m := New(sample(), Options{PreselectAll: true})
	// Cursor 0 is the apt header; toggling clears the fully-selected group.
	m.toggle()
	if m.selected["a1"] || m.selected["a2"] {
		t.Errorf("group toggle should clear a fully-selected group: %v", m.selected)
	}
	// Toggle again selects the whole group.
	m.toggle()
	if !m.selected["a1"] || !m.selected["a2"] {
		t.Errorf("group toggle should re-select the group: %v", m.selected)
	}
	// Move to an item and toggle it individually.
	m.moveCursor(1) // a1
	m.toggle()
	if m.selected["a1"] {
		t.Errorf("item toggle should deselect a1")
	}
}

func TestBulkRespectsFilterScopeAndBulkFlag(t *testing.T) {
	m := New(sample(), Options{})
	m.bulk(func(bool) bool { return true }) // select all
	// i1 (not Bulk) and i2 (not selectable) excluded.
	if m.selected["i1"] || m.selected["i2"] {
		t.Errorf("select-all must skip non-bulk/unselectable: %v", m.selected)
	}
	if !m.selected["a1"] || !m.selected["a2"] || !m.selected["i3"] {
		t.Errorf("select-all should select every bulk item: %v", m.selected)
	}
	// Filter to "curl", then clear within scope — only a1 is dropped.
	m.filter = "curl"
	m.bulk(func(bool) bool { return false })
	if m.selected["a1"] {
		t.Errorf("filtered none should drop a1")
	}
	if !m.selected["a2"] || !m.selected["i3"] {
		t.Errorf("items outside the filter must stay selected: %v", m.selected)
	}
}

func TestInvert(t *testing.T) {
	m := New(sample(), Options{})
	m.selected["a1"] = true
	m.bulk(func(cur bool) bool { return !cur })
	if m.selected["a1"] {
		t.Error("invert should deselect a1")
	}
	if !m.selected["a2"] || !m.selected["i3"] {
		t.Errorf("invert should select previously-unselected bulk items: %v", m.selected)
	}
}

func TestCollapseHidesItemsKeepsHeader(t *testing.T) {
	m := New(sample(), Options{})
	m.cursor = 0 // apt header
	m.collapse()
	// Header (row 0) visible; its items (rows 1,2) hidden.
	if !m.visibleRow(0) {
		t.Error("collapsed group header should stay visible")
	}
	if m.visibleRow(1) || m.visibleRow(2) {
		t.Error("collapsed group items should be hidden")
	}
}

func TestFilterCursorSnapsToMatch(t *testing.T) {
	m := New(sample(), Options{})
	m.filter = "redis"
	m.onFilterChanged()
	if m.rows[m.cursor].header || m.rows[m.cursor].item.Primary != "redis" {
		t.Errorf("cursor should snap to 'redis', got row %d", m.cursor)
	}
	// Headers for groups with no match should be hidden.
	if m.visibleRow(0) { // apt header, no "redis" match
		t.Error("apt header should be hidden when filter matches only infra")
	}
}

func TestWindowKeepsCursorVisible(t *testing.T) {
	m := New(sample(), Options{})
	m.height = 7 // force scrolling given chrome + indicators
	m.cursor = m.lastLandable()
	window, _, _ := m.windowFor(m.visibleIndices(), 4)
	found := false
	for _, idx := range window {
		if idx == m.cursor {
			found = true
		}
	}
	if !found {
		t.Errorf("cursor %d not in window %v", m.cursor, window)
	}
}

func TestResultOrderAndQuit(t *testing.T) {
	m := New(sample(), Options{})
	m.selected["i3"] = true
	m.selected["a2"] = true
	m.confirmed = true
	got := m.Result().Selected
	want := []string{"a2", "i3"} // display order
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Result order = %v, want %v", got, want)
	}

	q := New(sample(), Options{})
	q.quitting = true
	if !q.Result().Quit {
		t.Error("expected quit result")
	}
}

func TestUpdateKeyWiring(t *testing.T) {
	m := New(sample(), Options{PreselectAll: true})
	start := m.cursor
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.cursor == start {
		t.Error("down should move the cursor")
	}
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if !m.filtering {
		t.Error("'/' should enter filter mode")
	}
}

func TestRenderHeaderAndItem(t *testing.T) {
	m := New(sample(), Options{PreselectAll: true})
	h := m.renderHeader("apt")
	if !strings.Contains(h, "System Packages (apt)") || !strings.Contains(h, "✓") {
		t.Errorf("header render missing title/all-selected mark: %q", h)
	}
	kernel := Item{ID: "k", Primary: "linux-image", Secondary: "1 → 2", Selectable: true,
		Badges: []Badge{{Text: "KERNEL"}}}
	out := m.renderItem(row{item: kernel})
	if !strings.Contains(out, "linux-image") || !strings.Contains(out, "KERNEL") {
		t.Errorf("item render missing name/badge: %q", out)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 22); got != "short" {
		t.Errorf("no truncation expected, got %q", got)
	}
	long := truncate("a-very-long-package-name-indeed", 10)
	if !strings.HasSuffix(long, "…") {
		t.Errorf("expected ellipsis, got %q", long)
	}
}

func TestViewSmoke(t *testing.T) {
	m := New(sample(), Options{Title: "Available Updates", PreselectAll: true})
	// Should not panic at various sizes, including tiny ones that force scrolling.
	for _, sz := range [][2]int{{0, 0}, {80, 24}, {40, 6}, {120, 3}} {
		m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		_ = m.View()
	}
}
