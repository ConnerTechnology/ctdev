package diagnose

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

const (
	// clockWarn is well past anything latency or a coarse Date header could
	// account for, but short of breaking things.
	clockWarn = 60 * time.Second
	// clockFail is where TLS certificate validity windows, Kerberos tickets,
	// and time-based one-time passwords all start rejecting this machine —
	// which the user experiences as "every website is broken".
	clockFail = 5 * time.Minute
)

// checkClock compares the system clock against a server's Date header.
//
// It deliberately uses plain HTTP: a badly wrong clock breaks TLS certificate
// validation, so asking over HTTPS would fail for the very reason we're trying
// to detect. This also needs no tools and no root, so it behaves the same on
// all three platforms.
func checkClock(ctx context.Context, _ Facts) Result {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, captivePortalURL, nil)
	if err != nil {
		return skipf("could not build the probe request: %v", err)
	}
	resp, err := probeClient().Do(req)
	if err != nil {
		return skipf("no reachable server to compare the clock against")
	}
	defer resp.Body.Close()

	served, err := http.ParseTime(resp.Header.Get("Date"))
	if err != nil {
		return skipf("the server did not return a usable Date header")
	}
	return clockVerdict(time.Since(served))
}

// clockVerdict takes the signed offset between this machine and real time.
// Positive means the machine is ahead.
func clockVerdict(skew time.Duration) Result {
	signed := int(skew.Seconds())
	ahead := skew > 0
	if skew < 0 {
		skew = -skew
	}
	direction := "behind"
	if ahead {
		direction = "ahead"
	}

	var res Result
	switch {
	case skew >= clockFail:
		res = failf("Turn on automatic time in the date and time settings. Until it's fixed, secure sites will keep failing.",
			"the clock is %s %s — this breaks HTTPS on every site, and reads as 'the internet is down'",
			roundSkew(skew), direction)

	case skew >= clockWarn:
		res = warnf("Enable automatic time sync so it doesn't drift further.",
			"the clock is %s %s", roundSkew(skew), direction)

	default:
		res = okf("accurate to within %s", roundSkew(skew))
	}
	res.Data = map[string]string{DataClockSkewSec: strconv.Itoa(signed)}
	return res
}

// roundSkew formats an offset at a precision a person would use.
func roundSkew(d time.Duration) string {
	switch {
	case d >= time.Hour:
		return d.Round(time.Minute).String()
	case d >= time.Minute:
		return d.Round(time.Second).String()
	default:
		return d.Round(time.Second).String()
	}
}
