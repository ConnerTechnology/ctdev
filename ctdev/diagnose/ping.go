package diagnose

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"time"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
)

// PingResult summarizes a short ping run.
type PingResult struct {
	Sent, Received int
	LossPercent    float64
	AvgRTT         time.Duration
	MaxRTT         time.Duration
	// Jitter is mdev on Linux, stddev on BSD. Windows doesn't report it.
	Jitter time.Duration
}

// Lost reports whether nothing came back at all.
func (p PingResult) Lost() bool { return p.Received == 0 }

var errPingUnavailable = errors.New("no usable ping output")

// ping runs a short, bounded ping. It shells out rather than opening a raw
// socket because `ping` is present on every platform we support and needs no
// privileges there, whereas raw ICMP would need root on a machine we're only
// visiting.
func ping(ctx context.Context, info platform.Info, host string, count int) (PingResult, error) {
	if !commandExists("ping") {
		return PingResult{}, errPingUnavailable
	}

	var args []string
	switch info.OS {
	case platform.Windows:
		args = []string{"-n", strconv.Itoa(count), "-w", "1000", host}
	case platform.MacOS:
		// BSD ping takes -W in milliseconds; iputils takes seconds. Getting
		// this backwards makes every probe either instant or interminable.
		args = []string{"-c", strconv.Itoa(count), "-W", "1000", "-q", host}
	default:
		args = []string{"-c", strconv.Itoa(count), "-W", "1", "-q", host}
	}

	// Ping exits non-zero when every packet is lost, and that's a result, not
	// an error — parse the output either way.
	out, _ := captureErr(ctx, "ping", args...)
	if info.OS == platform.Windows {
		return parsePingWindows(out)
	}
	return parsePingUnix(out)
}

var (
	// Matches both iputils ("3 packets transmitted, 3 received, 0% packet
	// loss") and BSD ("3 packets transmitted, 3 packets received, 0.0% packet
	// loss").
	unixCountsRe = regexp.MustCompile(`(\d+) packets transmitted, (\d+)(?: packets)? received`)
	unixLossRe   = regexp.MustCompile(`([\d.]+)% packet loss`)
	// "rtt min/avg/max/mdev = 15.529/16.635/17.705/0.888 ms" on Linux,
	// "round-trip min/avg/max/stddev = ..." on macOS.
	unixRTTRe = regexp.MustCompile(`(?:rtt|round-trip) min/avg/max/(?:mdev|stddev) = ([\d.]+)/([\d.]+)/([\d.]+)/([\d.]+)`)
)

func parsePingUnix(out string) (PingResult, error) {
	m := unixCountsRe.FindStringSubmatch(out)
	if m == nil {
		return PingResult{}, errPingUnavailable
	}
	var p PingResult
	p.Sent, _ = strconv.Atoi(m[1])
	p.Received, _ = strconv.Atoi(m[2])

	if lm := unixLossRe.FindStringSubmatch(out); lm != nil {
		p.LossPercent, _ = strconv.ParseFloat(lm[1], 64)
	} else if p.Sent > 0 {
		p.LossPercent = float64(p.Sent-p.Received) / float64(p.Sent) * 100
	}

	// The RTT line is absent entirely when every packet is lost.
	if rm := unixRTTRe.FindStringSubmatch(out); rm != nil {
		p.AvgRTT = millis(rm[2])
		p.MaxRTT = millis(rm[3])
		p.Jitter = millis(rm[4])
	}
	return p, nil
}

var (
	// "    Packets: Sent = 4, Received = 4, Lost = 0 (0% loss),"
	winCountsRe = regexp.MustCompile(`Sent = (\d+), Received = (\d+), Lost = (\d+)`)
	// "    Minimum = 14ms, Maximum = 17ms, Average = 15ms"
	winRTTRe = regexp.MustCompile(`Minimum = (\d+)ms, Maximum = (\d+)ms, Average = (\d+)ms`)
)

func parsePingWindows(out string) (PingResult, error) {
	m := winCountsRe.FindStringSubmatch(out)
	if m == nil {
		return PingResult{}, errPingUnavailable
	}
	var p PingResult
	p.Sent, _ = strconv.Atoi(m[1])
	p.Received, _ = strconv.Atoi(m[2])
	if p.Sent > 0 {
		p.LossPercent = float64(p.Sent-p.Received) / float64(p.Sent) * 100
	}
	if rm := winRTTRe.FindStringSubmatch(out); rm != nil {
		p.MaxRTT = millis(rm[2])
		p.AvgRTT = millis(rm[3])
	}
	return p, nil
}

// millis parses a millisecond count that may carry a fraction.
func millis(s string) time.Duration {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return time.Duration(f * float64(time.Millisecond))
}
