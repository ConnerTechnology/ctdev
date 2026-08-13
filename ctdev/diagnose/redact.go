package diagnose

import (
	"regexp"
	"strings"
)

// macPattern matches a colon- or hyphen-separated MAC address. BSSIDs are the
// main target: a BSSID paired with an SSID can be looked up in public
// wardriving databases to locate the building the report was taken in, which
// is the sharpest privacy edge in an otherwise mundane report.
var macPattern = regexp.MustCompile(`\b([0-9a-fA-F]{2}[:-]){5}[0-9a-fA-F]{2}\b`)

const masked = "[redacted]"

// Redact returns a copy of the report with the identifiers that would locate
// or identify this network removed: the SSID, any MAC or BSSID, and the
// connection's public address.
//
// It deliberately does not mask every public address it sees. Naming 1.1.1.1
// and 8.8.8.8 is what makes "your DNS server is down, the public ones work"
// legible, and scrubbing them would leave a report nobody can act on. Only
// *this connection's* public address — which identifies the subscriber line —
// is removed.
//
// The hostname is also kept. It isn't a secret to whoever owns the machine,
// and when you're carrying reports from several machines it's the only thing
// telling them apart.
func Redact(r Report) Report {
	secrets := collectIdentifiers(r)

	out := r
	out.Findings = make([]Finding, len(r.Findings))
	for i, fd := range r.Findings {
		fd.Title = scrub(fd.Title, secrets)
		fd.Detail = scrub(fd.Detail, secrets)
		fd.Action = scrub(fd.Action, secrets)
		out.Findings[i] = fd
	}

	out.Results = make(map[string]Result, len(r.Results))
	for id, res := range r.Results {
		res.Detail = scrub(res.Detail, secrets)
		res.Advice = scrub(res.Advice, secrets)
		if len(res.Data) > 0 {
			data := make(map[string]string, len(res.Data))
			for k, v := range res.Data {
				data[k] = scrub(v, secrets)
			}
			res.Data = data
		}
		out.Results[id] = res
	}
	return out
}

// collectIdentifiers gathers the exact strings worth removing, taken from the
// checks that know them rather than guessed from the text.
func collectIdentifiers(r Report) []string {
	var secrets []string
	add := func(s string) {
		// Very short values would match far too much prose.
		if len(s) > 2 {
			secrets = append(secrets, s)
		}
	}

	if wifi, present := r.Results["link.wifi"]; present {
		add(wifi.Data["ssid"])
		add(wifi.Data["bssid"])
	}
	if pub, present := r.Results["net.public"]; present {
		add(pub.Data["public_ip"])
	}
	return secrets
}

func scrub(s string, secrets []string) string {
	if s == "" {
		return s
	}
	for _, secret := range secrets {
		s = strings.ReplaceAll(s, secret, masked)
	}
	return macPattern.ReplaceAllString(s, masked)
}
