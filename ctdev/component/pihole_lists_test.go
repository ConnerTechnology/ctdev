package component

import (
	"bytes"
	"testing"

	"github.com/ConnerTechnology/dotfiles/ctdev/piholelists"
)

// The embedded lists.toml is what `ctdev pihole sync` applies to a Pi-hole, so
// a typo in it would only surface on the node. Parse it here instead, and
// require it to be byte-identical to what `ctdev pihole export` would write —
// otherwise an export right after a sync shows a spurious diff.
func TestPiholeListsFileIsCanonical(t *testing.T) {
	raw, err := Configs.ReadFile("configs/pihole/lists.toml")
	if err != nil {
		t.Fatalf("read embedded lists.toml: %v", err)
	}
	lists, err := piholelists.Parse(raw)
	if err != nil {
		t.Fatalf("parse lists.toml: %v", err)
	}
	if len(lists) == 0 {
		t.Fatal("lists.toml is empty")
	}
	if !bytes.Equal(raw, piholelists.Encode(lists)) {
		t.Error("lists.toml is not in canonical form; regenerate it with 'ctdev pihole export'")
	}
}
