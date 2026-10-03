package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/ConnerTechnology/ctdev/ctdev/linear"
	"github.com/spf13/cobra"
)

// The `ctdev linear` commands are plumbing for Claude Code and for agents, not
// for people, so they are hidden. A person sets a repo up with
// `ctdev configure linear`.

var flagLinearWorkspace string

// newLinearClient is swapped in tests to point at an httptest server.
var newLinearClient = linear.NewClient

var linearCmd = &cobra.Command{
	Use:    "linear",
	Short:  "Linear app tokens for Claude Code (plumbing)",
	Hidden: true,
}

var linearMCPHeadersCmd = &cobra.Command{
	Use:   "mcp-headers --workspace <name>",
	Short: "Print the Authorization header for the Linear MCP server",
	Long: "The headersHelper `ctdev configure linear` writes into a repo's .mcp.json. Prints " +
		`{"Authorization":"Bearer <token>"}` + " and nothing else on stdout, using the cached " +
		"app token for the workspace, or a new one when it is stale or Linear rejects it.",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return linearMCPHeaders(cmd, flagLinearWorkspace, cmd.OutOrStdout())
	},
}

var linearGraphQLCmd = &cobra.Command{
	Use:   "graphql --workspace <name> QUERY [VARIABLES_JSON]",
	Short: "Send one GraphQL request to Linear as the app",
	Long: "Sends one GraphQL request as the workspace's app and prints the response body, for " +
		"the operations the Linear MCP has no tool for. Exits non-zero when the HTTP status is not 200.",
	Args:   cobra.RangeArgs(1, 2),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var vars json.RawMessage
		if len(args) == 2 {
			vars = json.RawMessage(args[1])
		}
		return linearGraphQL(cmd, flagLinearWorkspace, args[0], vars, cmd.OutOrStdout())
	},
}

func init() {
	for _, c := range []*cobra.Command{linearMCPHeadersCmd, linearGraphQLCmd} {
		c.Flags().StringVar(&flagLinearWorkspace, "workspace", "", "Linear workspace name (as set up by 'ctdev configure linear')")
		_ = c.MarkFlagRequired("workspace")
		linearCmd.AddCommand(c)
	}
	rootCmd.AddCommand(linearCmd)
}

// linearHelperError is what Claude Code shows when the headersHelper fails, so
// it names the workspace and the way to fix it.
func linearHelperError(workspace string, err error) error {
	return fmt.Errorf("linear workspace %q: %v; run 'ctdev configure linear' in this repo to check or replace the credentials", workspace, err)
}

func linearMCPHeaders(cmd *cobra.Command, workspace string, out io.Writer) error {
	if err := linear.ValidateWorkspace(workspace); err != nil {
		return linearHelperError(workspace, err)
	}
	token, err := newLinearClient().Token(cmdContext(cmd), workspace)
	if err != nil {
		return linearHelperError(workspace, err)
	}
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	return enc.Encode(map[string]string{"Authorization": "Bearer " + token})
}

func linearGraphQL(cmd *cobra.Command, workspace, query string, vars json.RawMessage, out io.Writer) error {
	if err := linear.ValidateWorkspace(workspace); err != nil {
		return linearHelperError(workspace, err)
	}
	if vars != nil && !json.Valid(vars) {
		return fmt.Errorf("VARIABLES_JSON is not valid JSON")
	}
	body, status, err := newLinearClient().GraphQL(cmdContext(cmd), workspace, query, vars)
	if err != nil {
		return linearHelperError(workspace, err)
	}
	fmt.Fprintf(out, "%s\n", body)
	if status != http.StatusOK {
		return fmt.Errorf("GraphQL request failed with HTTP %d", status)
	}
	return nil
}
