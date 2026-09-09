package piholelists

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const fileHeader = "# Pi-hole lists — managed by `ctdev pihole sync` (apply) and `ctdev pihole export` (record).\n" +
	"# Groups other than Default are created if missing; which clients belong to a group is set in the web UI.\n"

// tomlEntry mirrors one inline table. The identity keys share a struct so a
// misplaced one (a "url" under allow) is caught by Parse with a useful message
// rather than silently ignored. Pointers distinguish "absent" from "set to the
// zero value", which is what the defaults hang on.
type tomlEntry struct {
	URL     string    `toml:"url"`
	Domain  string    `toml:"domain"`
	Pattern string    `toml:"pattern"`
	Comment string    `toml:"comment"`
	Groups  *[]string `toml:"groups"`
	Enabled *bool     `toml:"enabled"`
}

type tomlFile struct {
	Adlists    []tomlEntry `toml:"adlists"`
	Allow      []tomlEntry `toml:"allow"`
	Deny       []tomlEntry `toml:"deny"`
	AllowRegex []tomlEntry `toml:"allow_regex"`
	DenyRegex  []tomlEntry `toml:"deny_regex"`
}

// Parse reads a lists.toml file. Entries come back in file order with defaults
// filled in: groups default to Default, enabled defaults to true.
func Parse(data []byte) (Lists, error) {
	var f tomlFile
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, err
	}

	var out Lists
	seen := map[[2]string]bool{}
	for _, kind := range Kinds {
		for i, raw := range f.section(kind) {
			entry, err := raw.toEntry(kind)
			if err != nil {
				return nil, fmt.Errorf("%s entry %d: %w", kind.Key(), i+1, err)
			}
			id := [2]string{kind.Key(), entry.Key}
			if seen[id] {
				return nil, fmt.Errorf("%s entry %d: duplicate %s %q", kind.Key(), i+1, kind.Field(), entry.Key)
			}
			seen[id] = true
			out = append(out, entry)
		}
	}
	return out, nil
}

func (inst tomlFile) section(k Kind) []tomlEntry {
	switch k {
	case Adlist:
		return inst.Adlists
	case Allow:
		return inst.Allow
	case Deny:
		return inst.Deny
	case AllowRegex:
		return inst.AllowRegex
	default:
		return inst.DenyRegex
	}
}

func (inst tomlEntry) toEntry(kind Kind) (Entry, error) {
	keys := map[string]string{"url": inst.URL, "domain": inst.Domain, "pattern": inst.Pattern}
	for name, value := range keys {
		if value != "" && name != kind.Field() {
			return Entry{}, fmt.Errorf("has %q but %s entries are keyed by %q", name, kind.Key(), kind.Field())
		}
	}
	key := strings.TrimSpace(keys[kind.Field()])
	if key == "" {
		return Entry{}, fmt.Errorf("missing %q", kind.Field())
	}

	groups := []string{DefaultGroup}
	if inst.Groups != nil {
		for _, g := range *inst.Groups {
			if strings.TrimSpace(g) == "" {
				return Entry{}, fmt.Errorf("empty group name")
			}
		}
		groups = *inst.Groups
	}

	return Entry{
		Kind:    kind,
		Key:     key,
		Comment: inst.Comment,
		Groups:  normalizeGroups(groups),
		Enabled: inst.Enabled == nil || *inst.Enabled,
	}, nil
}

// Encode writes lists.toml. go-toml would emit multi-line [[allow]] blocks, so
// this writes one inline table per line instead: a list of a few hundred
// domains then reads as one entry per line in a git diff. Fields at their
// default (enabled, the Default group, an empty comment) are omitted.
func Encode(lists Lists) []byte {
	sorted := slices.Clone(lists)
	sorted.Sort()

	var b strings.Builder
	b.WriteString(fileHeader)
	for _, kind := range Kinds {
		fmt.Fprintf(&b, "\n%s = [\n", kind.Key())
		for _, e := range sorted.OfKind(kind) {
			b.WriteString(encodeEntry(e))
		}
		b.WriteString("]\n")
	}
	return []byte(b.String())
}

func encodeEntry(e Entry) string {
	fields := []string{e.Kind.Field() + " = " + encodeKey(e)}
	if e.Comment != "" {
		fields = append(fields, "comment = "+basicString(e.Comment))
	}
	if !isDefaultGroups(e.Groups) {
		quoted := make([]string, len(e.Groups))
		for i, g := range e.Groups {
			quoted[i] = basicString(g)
		}
		fields = append(fields, "groups = ["+strings.Join(quoted, ", ")+"]")
	}
	if !e.Enabled {
		fields = append(fields, "enabled = false")
	}
	return "  { " + strings.Join(fields, ", ") + " },\n"
}

// encodeKey prefers a literal string for regex patterns, where a basic string
// would double every backslash and make the pattern hard to read.
func encodeKey(e Entry) string {
	isRegex := e.Kind == AllowRegex || e.Kind == DenyRegex
	if isRegex && literalSafe(e.Key) {
		return "'" + e.Key + "'"
	}
	return basicString(e.Key)
}

func literalSafe(s string) bool {
	if strings.ContainsRune(s, '\'') {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func basicString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
