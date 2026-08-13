package diagnose

import (
	"context"
	"net/http"
	"time"
)

func checkLocalAddress(_ context.Context, f Facts) Result {
	switch {
	case !f.LocalIP.IsValid():
		return failf("Check that Wi-Fi is on and the cable is seated, then reconnect to the network.",
			"no usable address — this machine isn't on a network")

	case LinkLocalIPv4(f.LocalIP):
		// 169.254.x is what an OS assigns itself when DHCP gets no answer.
		// It's the single most diagnostic address a machine can have.
		return failf("Reboot the router. If other devices work, forget this network on this machine and rejoin.",
			"self-assigned %s on %s — the router never handed out an address (DHCP failed)",
			f.LocalIP, describeLink(f))

	case IsPrivate(f.LocalIP):
		return okf("%s on %s", f.LocalIP, describeLink(f))

	default:
		// A public address directly on the interface is legitimate on a
		// server, and worth saying out loud on a laptop.
		return infof("%s on %s — a public address, not behind NAT", f.LocalIP, describeLink(f))
	}
}

func describeLink(f Facts) string {
	if f.Iface == "" {
		return "an unknown interface"
	}
	switch {
	case f.IsWiFi:
		return f.Iface + " (Wi-Fi)"
	default:
		return f.Iface + " (wired)"
	}
}

// captivePortalURL is the endpoint Android uses for the same purpose. It is
// deliberately plain HTTP: a captive portal has to intercept it to redirect us,
// which is exactly the signal we're looking for.
const captivePortalURL = "http://connectivitycheck.gstatic.com/generate_204"

func checkCaptivePortal(ctx context.Context, _ Facts) Result {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, captivePortalURL, nil)
	if err != nil {
		return skipf("could not build the probe request: %v", err)
	}

	resp, err := probeClient().Do(req)
	if err != nil {
		return failf("Check whether the gateway responds. If it does, the problem is upstream — power-cycle the modem.",
			"no answer from the internet — %s", netReason(err))
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNoContent:
		return okf("reachable")

	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		res := failf("Open a browser and complete the network's sign-in page, then run this again.",
			"redirected to %s — this network wants a browser sign-in first (captive portal)",
			resp.Header.Get("Location"))
		// Only a redirect proves a portal. A plain connection failure fails
		// this check too, and must not be mistaken for one.
		res.Data = map[string]string{DataCaptivePortal: "yes"}
		return res

	default:
		// A 200 where a 204 was promised means something rewrote the
		// response — a portal splash page, or a filtering middlebox.
		return warnf("Open a browser and see what loads. A sign-in page means a captive portal; anything else is a filtering proxy.",
			"expected 204, got %s — something is intercepting plain HTTP", resp.Status)
	}
}

// probeClient is for reachability probes, not downloads: it never follows
// redirects, because a redirect is the finding.
func probeClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
