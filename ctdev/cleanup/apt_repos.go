package cleanup

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RepoDup is a repository (URI + suite) declared in more than one source file.
type RepoDup struct {
	Key   string // "uri suite"
	Files []string
}

// auditAPTRepos finds repositories declared in multiple files under dir. Unlike
// a naive line-by-line diff, it understands both legacy one-line `.list` entries
// and deb822 `.sources` stanzas, so shared deb822 keys (Types: deb, Components:
// main, …) don't read as false-positive duplicates.
func auditAPTRepos(dir string) []RepoDup {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	seen := map[string]map[string]bool{} // key -> set of files
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var keys []string
		switch {
		case strings.HasSuffix(name, ".list"):
			keys = parseListRepos(string(data))
		case strings.HasSuffix(name, ".sources"):
			keys = parseSourcesRepos(string(data))
		default:
			continue
		}
		for _, k := range keys {
			if seen[k] == nil {
				seen[k] = map[string]bool{}
			}
			seen[k][name] = true
		}
	}

	var dups []RepoDup
	for key, files := range seen {
		if len(files) > 1 {
			var fs []string
			for f := range files {
				fs = append(fs, f)
			}
			sort.Strings(fs)
			dups = append(dups, RepoDup{Key: key, Files: fs})
		}
	}
	sort.Slice(dups, func(i, j int) bool { return dups[i].Key < dups[j].Key })
	return dups
}

// parseListRepos extracts "uri suite" keys from a legacy one-line-per-repo file.
func parseListRepos(content string) []string {
	var keys []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 || (fields[0] != "deb" && fields[0] != "deb-src") {
			continue
		}
		i := 1
		// Skip an optional [opt=val ...] block.
		if strings.HasPrefix(fields[i], "[") {
			for i < len(fields) && !strings.HasSuffix(fields[i], "]") {
				i++
			}
			i++ // move past the closing token
		}
		if i+1 >= len(fields) {
			continue
		}
		keys = append(keys, fields[i]+" "+fields[i+1])
	}
	return keys
}

// parseSourcesRepos extracts "uri suite" keys from a deb822 .sources file,
// expanding the cross-product of URIs and Suites in each enabled stanza.
func parseSourcesRepos(content string) []string {
	var keys []string
	for _, stanza := range strings.Split(content, "\n\n") {
		var uris, suites []string
		enabled := true
		for _, line := range strings.Split(stanza, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, val, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			val = strings.TrimSpace(val)
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "uris":
				uris = strings.Fields(val)
			case "suites":
				suites = strings.Fields(val)
			case "enabled":
				enabled = !strings.EqualFold(val, "no") && !strings.EqualFold(val, "false")
			}
		}
		if !enabled {
			continue
		}
		for _, u := range uris {
			for _, s := range suites {
				keys = append(keys, u+" "+s)
			}
		}
	}
	return keys
}
