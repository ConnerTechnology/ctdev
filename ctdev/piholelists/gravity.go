package piholelists

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Executor runs SQL against a Pi-hole's gravity.db. Query returns the rows as
// the JSON array `pihole-FTL sqlite3 -json` prints; Exec runs a statement
// script for its effect. Splitting reads from writes keeps the write path (and
// its sudo/docker exec) out of anything that only inspects the Pi-hole.
type Executor interface {
	Query(ctx context.Context, sql string) ([]byte, error)
	Exec(ctx context.Context, sql string) error
}

// Each row carries its groups as a comma-joined name list, so reading the whole
// Pi-hole costs three queries instead of one per entry.
const (
	domainlistQuery = `SELECT d.type AS type, d.domain AS domain, d.enabled AS enabled, d.comment AS comment, ` +
		`(SELECT group_concat(g.name) FROM domainlist_by_group dg JOIN "group" g ON g.id = dg.group_id WHERE dg.domainlist_id = d.id) AS groups ` +
		`FROM domainlist d;`
	adlistQuery = `SELECT a.address AS address, a.enabled AS enabled, a.comment AS comment, ` +
		`(SELECT group_concat(g.name) FROM adlist_by_group ag JOIN "group" g ON g.id = ag.group_id WHERE ag.adlist_id = a.id) AS groups ` +
		`FROM adlist a;`
	groupQuery = `SELECT name FROM "group" ORDER BY name;`
)

type domainRow struct {
	Type    int     `json:"type"`
	Domain  string  `json:"domain"`
	Enabled int     `json:"enabled"`
	Comment *string `json:"comment"`
	Groups  *string `json:"groups"`
}

type adlistRow struct {
	Address string  `json:"address"`
	Enabled int     `json:"enabled"`
	Comment *string `json:"comment"`
	Groups  *string `json:"groups"`
}

// ReadLive reads every list out of gravity.db, sorted the same way Encode
// writes them.
func ReadLive(ctx context.Context, ex Executor) (Lists, error) {
	var domains []domainRow
	if err := queryJSON(ctx, ex, domainlistQuery, &domains); err != nil {
		return nil, fmt.Errorf("read domainlist: %w", err)
	}
	var adlists []adlistRow
	if err := queryJSON(ctx, ex, adlistQuery, &adlists); err != nil {
		return nil, fmt.Errorf("read adlist: %w", err)
	}

	out := make(Lists, 0, len(domains)+len(adlists))
	for _, r := range adlists {
		out = append(out, Entry{
			Kind:    Adlist,
			Key:     r.Address,
			Comment: deref(r.Comment),
			Groups:  splitGroups(r.Groups),
			Enabled: r.Enabled != 0,
		})
	}
	for _, r := range domains {
		kind, ok := kindForType(r.Type)
		if !ok {
			// A future Pi-hole type this ctdev doesn't model: leave it alone
			// rather than have Diff propose deleting it.
			continue
		}
		out = append(out, Entry{
			Kind:    kind,
			Key:     r.Domain,
			Comment: deref(r.Comment),
			Groups:  splitGroups(r.Groups),
			Enabled: r.Enabled != 0,
		})
	}
	out.Sort()
	return out, nil
}

// ReadGroups returns the names of every group defined on the Pi-hole, so
// ApplySQL knows which ones it has to create.
func ReadGroups(ctx context.Context, ex Executor) ([]string, error) {
	var rows []struct {
		Name string `json:"name"`
	}
	if err := queryJSON(ctx, ex, groupQuery, &rows); err != nil {
		return nil, fmt.Errorf("read groups: %w", err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Name)
	}
	return out, nil
}

// queryJSON runs a query and decodes it. An empty result set prints nothing at
// all rather than "[]", which is not valid JSON.
func queryJSON(ctx context.Context, ex Executor, sql string, dest any) error {
	out, err := ex.Query(ctx, sql)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		return nil
	}
	return json.Unmarshal(out, dest)
}

func kindForType(t int) (Kind, bool) {
	for _, k := range Kinds {
		if k != Adlist && k.DomainlistType() == t {
			return k, true
		}
	}
	return Adlist, false
}

func splitGroups(joined *string) []string {
	if joined == nil {
		return nil
	}
	return normalizeGroups(strings.Split(*joined, ","))
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
