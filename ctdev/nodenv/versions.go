package nodenv

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// stable matches a Node release with no pre-release suffix. node-build's lists
// also carry graal, chakracore and nightly builds, which ctdev never offers.
var stable = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// Major is the major of "24.22.3", 0 when v has none.
func Major(v string) int {
	m, _, _ := strings.Cut(strings.TrimPrefix(v, "v"), ".")
	n, _ := strconv.Atoi(m)
	return n
}

// Compare orders two dotted versions numerically: -1, 0 or 1.
func Compare(a, b string) int {
	pa, pb := strings.Split(strings.TrimPrefix(a, "v"), "."), strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < max(len(pa), len(pb)); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return strings.Compare(a, b)
}

// ParseList keeps the stable releases from `nodenv install --list` or
// `--list-all` output, newest first.
func ParseList(out string) []string {
	var vs []string
	for _, line := range strings.Split(out, "\n") {
		if v := strings.TrimSpace(line); stable.MatchString(v) {
			vs = append(vs, v)
		}
	}
	slices.SortFunc(vs, func(a, b string) int { return Compare(b, a) })
	return slices.Compact(vs)
}

// LatestPerMajor is the newest release of each major in list, newest major
// first.
func LatestPerMajor(list []string) []string {
	var out []string
	for _, v := range ParseList(strings.Join(list, "\n")) {
		if len(out) == 0 || Major(out[len(out)-1]) != Major(v) {
			out = append(out, v)
		}
	}
	return out
}

// LatestOfMajor is the newest release of major in list, "" when it has none.
func LatestOfMajor(list []string, major int) string {
	for _, v := range LatestPerMajor(list) {
		if Major(v) == major {
			return v
		}
	}
	return ""
}

// Resolve turns what a user typed into a release from list: a bare major
// ("22") is that major's newest release, a full version must be in list.
func Resolve(list []string, arg string) (string, bool) {
	arg = strings.TrimPrefix(arg, "v")
	if !strings.Contains(arg, ".") {
		n, err := strconv.Atoi(arg)
		if err != nil {
			return "", false
		}
		v := LatestOfMajor(list, n)
		return v, v != ""
	}
	return arg, slices.Contains(list, arg)
}

// Release is one entry of nodejs.org's dist index. LTS is the codename, ""
// when the release is not LTS.
type Release struct {
	Version string
	LTS     string
}

// ParseIndex reads https://nodejs.org/dist/index.json, whose "lts" field is
// false or the line's codename.
func ParseIndex(data []byte) ([]Release, error) {
	var raw []struct {
		Version string          `json:"version"`
		LTS     json.RawMessage `json:"lts"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse Node release index: %w", err)
	}
	rs := make([]Release, 0, len(raw))
	for _, r := range raw {
		var codename string
		_ = json.Unmarshal(r.LTS, &codename) // false leaves it empty
		rs = append(rs, Release{Version: strings.TrimPrefix(r.Version, "v"), LTS: codename})
	}
	return rs, nil
}

// LTSLine is the newest Node major in long-term support.
type LTSLine struct {
	Major    int
	Codename string
}

// PickLTS is the highest major whose newest stable release is LTS. A major
// turns LTS partway through its life (October for an even major), so a new
// even major is "Current" until then and must not be picked.
func PickLTS(rs []Release) (LTSLine, bool) {
	newest := map[int]Release{}
	for _, r := range rs {
		if !stable.MatchString(r.Version) {
			continue
		}
		m := Major(r.Version)
		if cur, ok := newest[m]; !ok || Compare(r.Version, cur.Version) > 0 {
			newest[m] = r
		}
	}
	var best LTSLine
	for m, r := range newest {
		if r.LTS != "" && m > best.Major {
			best = LTSLine{Major: m, Codename: r.LTS}
		}
	}
	return best, best.Major != 0
}

// Package is a global npm package installed under one Node version.
type Package struct {
	Name    string
	Version string
}

// Spec is the argument that reinstalls p at the same version.
func (p Package) Spec() string { return p.Name + "@" + p.Version }

// Specs joins the packages' specs for display.
func Specs(ps []Package) string {
	s := make([]string, len(ps))
	for i, p := range ps {
		s[i] = p.Spec()
	}
	return strings.Join(s, ", ")
}
