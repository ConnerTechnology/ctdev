package linear

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Team is one Linear team the app can see.
type Team struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// Status is what `ctdev configure linear` reports for a workspace.
type Status struct {
	Workspace      string
	HasCredentials bool
	// Connected is true when a token was obtained and Linear answered a query
	// with it.
	Connected bool
	Actor     string
	Teams     []Team
	// Err is why the workspace is not connected, nil when it is.
	Err error
}

// TeamKeys lists the teams' keys, in Linear's order.
func (s Status) TeamKeys() []string {
	keys := make([]string, 0, len(s.Teams))
	for _, t := range s.Teams {
		keys = append(keys, t.Key)
	}
	return keys
}

const statusQuery = `{ viewer { name } teams { nodes { key name } } }`

// CheckStatus reports whether a workspace has credentials, whether a token
// can be obtained with them, and who that token acts as.
func (c *Client) CheckStatus(ctx context.Context, workspace string) Status {
	st := Status{Workspace: workspace}
	if _, err := LoadCredentials(workspace); err != nil {
		st.Err = err
		return st
	}
	st.HasCredentials = true

	token, err := c.Token(ctx, workspace)
	if err != nil {
		st.Err = err
		return st
	}
	body, status, err := c.graphQLWithToken(ctx, token, statusQuery, nil)
	if err != nil {
		st.Err = err
		return st
	}
	var resp struct {
		Data struct {
			Viewer struct {
				Name string `json:"name"`
			} `json:"viewer"`
			Teams struct {
				Nodes []Team `json:"nodes"`
			} `json:"teams"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	jsonErr := json.Unmarshal(body, &resp)
	if status != http.StatusOK || jsonErr != nil || len(resp.Errors) > 0 {
		msg := fmt.Sprintf("Linear answered HTTP %d", status)
		var msgs []string
		for _, e := range resp.Errors {
			msgs = append(msgs, e.Message)
		}
		if len(msgs) > 0 {
			msg += " (" + strings.Join(msgs, "; ") + ")"
		}
		st.Err = errors.New(msg)
		return st
	}
	st.Connected = true
	st.Actor = resp.Data.Viewer.Name
	st.Teams = resp.Data.Teams.Nodes
	return st
}
