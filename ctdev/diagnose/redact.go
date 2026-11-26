package diagnose

import (
	"net/netip"
	"regexp"
	"strings"
)

// macPattern matches a colon- or hyphen-separated MAC address. BSSIDs are the
// main target: a BSSID paired with an SSID can be looked up in public
// wardriving databases to locate the building the report was taken in, which
// is the sharpest privacy edge in an otherwise mundane report.
var macPattern = regexp.MustCompile(`\b([0-9a-fA-F]{2}[:-]){5}[0-9a-fA-F]{2}\b`)

const masked = "[redacted]"

type redactor struct {
	literals []string
	patterns []*regexp.Regexp
}

func (rd redactor) scrub(s string) string {
	if s == "" {
		return s
	}
	for _, secret := range rd.literals {
		s = strings.ReplaceAll(s, secret, masked)
	}
	for _, pat := range rd.patterns {
		s = pat.ReplaceAllString(s, masked)
	}
	return macPattern.ReplaceAllString(s, masked)
}

func Redact(r Report) Report {
	rd := collectIdentifiers(r)

	out := r
	if out.Facts.LocalIP.IsValid() && !IsPrivate(out.Facts.LocalIP) {
		out.Facts.LocalIP = netip.Addr{}
	}
	out.Findings = make([]Finding, len(r.Findings))
	for i, fd := range r.Findings {
		fd.Title = rd.scrub(fd.Title)
		fd.Detail = rd.scrub(fd.Detail)
		fd.Action = rd.scrub(fd.Action)
		out.Findings[i] = fd
	}

	out.Results = make(map[string]Result, len(r.Results))
	for id, res := range r.Results {
		res.Detail = rd.scrub(res.Detail)
		res.Advice = rd.scrub(res.Advice)
		if len(res.Data) > 0 {
			data := make(map[string]string, len(res.Data))
			for k, v := range res.Data {
				if id == "link.wifi" && k == "ssid" && v != "" {
					data[k] = masked
					continue
				}
				data[k] = rd.scrub(v)
			}
			res.Data = data
		}
		out.Results[id] = res
	}
	return out
}

// collectIdentifiers gathers the exact strings worth removing, taken from the
// checks that know them rather than guessed from the text.
func collectIdentifiers(r Report) redactor {
	var rd redactor
	addLiteral := func(s string) {
		if len(s) > 2 {
			rd.literals = append(rd.literals, s)
		}
	}
	addSSID := func(s string) {
		if s == "" {
			return
		}
		// Short SSIDs (e.g. "net", "lan") would corrupt normal words like
		// "internet" if replaced by bare substring; use word boundaries instead.
		if len(s) < 5 {
			if len(s) > 2 {
				rd.patterns = append(rd.patterns, regexp.MustCompile(`\b`+regexp.QuoteMeta(s)+`\b`))
			}
			return
		}
		rd.literals = append(rd.literals, s)
	}

	if wifi, present := r.Results["link.wifi"]; present {
		addSSID(wifi.Data["ssid"])
		addLiteral(wifi.Data["bssid"])
	}
	if pub, present := r.Results["net.public"]; present {
		addLiteral(pub.Data["public_ip"])
	}
	if wan, present := r.Results["unifi.health"]; present {
		addLiteral(wan.Data["wan_ip"])
	}
	if r.Facts.LocalIP.IsValid() && !IsPrivate(r.Facts.LocalIP) {
		addLiteral(r.Facts.LocalIP.String())
	}
	return rd
}
