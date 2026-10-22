package linear

import (
	"testing"
)

func TestMCPJSONWorkspace(t *testing.T) {
	cases := []struct {
		doc  string
		ws   string
		want bool
	}{
		{`{"mcpServers":{"linear":{"headersHelper":"ctdev linear mcp-headers --workspace acme"}}}`, "acme", true},
		{`{"mcpServers":{"linear":{"headersHelper":"./scripts/linear-app.sh --mcp-headers"}}}`, "", false},
		{`{"mcpServers":{"other":{"headersHelper":"ctdev linear mcp-headers --workspace acme"}}}`, "", false},
		{`{"mcpServers":{"linear":{"headersHelper":"ctdev linear mcp-headers --workspace Bad_Name"}}}`, "", false},
		{`not json`, "", false},
		{``, "", false},
	}
	for _, tc := range cases {
		ws, ok := MCPJSONWorkspace([]byte(tc.doc))
		if ws != tc.ws || ok != tc.want {
			t.Errorf("%s: got %q, %v", tc.doc, ws, ok)
		}
	}
}

func TestMCPEntryRoundTrips(t *testing.T) {
	ws, ok := EntryWorkspace(MCPEntry("acme"))
	if !ok || ws != "acme" {
		t.Errorf("got %q, %v", ws, ok)
	}
	if _, ok := EntryWorkspace([]byte(`{"type":"http","url":"https://mcp.linear.app/mcp"}`)); ok {
		t.Error("an entry with no headersHelper is not ours")
	}
}
