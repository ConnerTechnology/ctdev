package cmd

import (
	"fmt"
	"io"

	"github.com/ConnerTechnology/ctdev/ctdev/jira"
	"github.com/spf13/cobra"
)

// The `ctdev jira` commands are plumbing for scripts, not for people, so they
// are hidden. A person sets a site up with `ctdev configure jira`.

var flagJiraSite string

var jiraCmd = &cobra.Command{
	Use:    "jira",
	Short:  "Jira API token for scripts (plumbing)",
	Hidden: true,
}

var jiraAuthHeaderCmd = &cobra.Command{
	Use:   "auth-header --site <name>",
	Short: "Print the HTTP Authorization header for Jira's REST API",
	Long: "Prints `Authorization: Basic <base64 of email:token>` and nothing else on stdout, from " +
		"the API token `ctdev configure jira` saved for the site, so a script can run " +
		"`curl -H \"$(ctdev jira auth-header --site <name>)\" https://<name>.atlassian.net/rest/api/3/...`. " +
		"It reads the saved file and makes no request.",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return jiraAuthHeader(flagJiraSite, cmd.OutOrStdout())
	},
}

func init() {
	jiraAuthHeaderCmd.Flags().StringVar(&flagJiraSite, "site", "", "Jira site name, the part before .atlassian.net (as set up by 'ctdev configure jira')")
	_ = jiraAuthHeaderCmd.MarkFlagRequired("site")
	jiraCmd.AddCommand(jiraAuthHeaderCmd)
	rootCmd.AddCommand(jiraCmd)
}

func jiraAuthHeader(site string, out io.Writer) error {
	creds, err := jira.LoadCredentials(site)
	if err != nil {
		return fmt.Errorf("jira site %q: %v; run 'ctdev configure jira' to save an API token", site, err)
	}
	_, err = fmt.Fprintf(out, "Authorization: %s\n", creds.AuthHeader())
	return err
}
