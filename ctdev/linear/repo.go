package linear

import (
	"encoding/json"
	"regexp"
	"strings"
)

// MCPJSONFile is the committed project MCP config, where ctdev wrote the
// linear server before it went to local scope (CTD-109). It is only read now.
const MCPJSONFile = ".mcp.json"

// ServerName is the MCP server's name in Claude Code.
const ServerName = "linear"

// MCPURL is Linear's hosted MCP server.
const MCPURL = "https://mcp.linear.app/mcp"

const headersHelperPrefix = "ctdev linear mcp-headers --workspace "

var headersHelperPattern = regexp.MustCompile(`^ctdev linear mcp-headers --workspace ([a-z0-9][a-z0-9-]*)$`)

// HeadersHelper is the server's headersHelper command for a workspace.
func HeadersHelper(workspace string) string {
	return headersHelperPrefix + workspace
}

// mcpServerEntry is the "linear" server for a workspace.
type mcpServerEntry struct {
	Type          string `json:"type"`
	URL           string `json:"url"`
	HeadersHelper string `json:"headersHelper"`
}

// MCPEntry is the "linear" server entry for a workspace, as indented JSON.
func MCPEntry(workspace string) json.RawMessage {
	b, _ := json.MarshalIndent(mcpServerEntry{"http", MCPURL, HeadersHelper(workspace)}, "", "  ")
	return b
}

// EntryWorkspace reads the workspace a linear server entry points at. ok is
// false when the entry's headersHelper is not ours.
func EntryWorkspace(entry json.RawMessage) (workspace string, ok bool) {
	var e struct {
		HeadersHelper string `json:"headersHelper"`
	}
	if json.Unmarshal(entry, &e) != nil {
		return "", false
	}
	m := headersHelperPattern.FindStringSubmatch(strings.TrimSpace(e.HeadersHelper))
	if m == nil {
		return "", false
	}
	return m[1], true
}

// MCPJSONWorkspace reads the workspace a repo's .mcp.json linear server points
// at. ok is false when there is no linear server or its headersHelper is not
// ours.
func MCPJSONWorkspace(mcpJSON []byte) (workspace string, ok bool) {
	var doc struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if json.Unmarshal(mcpJSON, &doc) != nil {
		return "", false
	}
	entry, found := doc.MCPServers[ServerName]
	if !found {
		return "", false
	}
	return EntryWorkspace(entry)
}

// HasMCPJSONServer reports whether a .mcp.json has a linear server of any
// kind, ctdev's or not.
func HasMCPJSONServer(mcpJSON []byte) bool {
	var doc struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if json.Unmarshal(mcpJSON, &doc) != nil {
		return false
	}
	_, ok := doc.MCPServers[ServerName]
	return ok
}
