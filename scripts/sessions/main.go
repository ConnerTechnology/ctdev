// Command sessions reads the agent sessions Entire recorded as checkpoints in this repo
// (refs/entire/checkpoints/), on this machine only: nothing is sent anywhere, and the checkpoints
// themselves are never pushed (docs/agents/goal-loop.md). scripts/sessions.sh builds and runs it.
//
//	sessions digest [--since YYYY-MM-DD] [--until YYYY-MM-DD]   signals for the daily retro
//	sessions search [--since YYYY-MM-DD] <term>...               where past sessions mention every term
//
// digest defaults to yesterday (local time); search to every checkpoint.
// Exit: 0 done (search: at least one match); 1 error (search: no match); 2 usage.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

func main() { os.Exit(run(os.Args[1:], gitStore{dir: "."}, time.Now())) }

func usage() int {
	fmt.Fprintln(os.Stderr, "usage: sessions digest [--since YYYY-MM-DD] [--until YYYY-MM-DD]\n       sessions search [--since YYYY-MM-DD] <term>...")
	return 2
}

func run(args []string, store Store, now time.Time) int {
	if len(args) == 0 {
		return usage()
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	sinceFlag := fs.String("since", "", "first day, YYYY-MM-DD, local time")
	untilFlag := fs.String("until", "", "day after the last, YYYY-MM-DD, local time")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	since, until := time.Time{}, today.AddDate(100, 0, 0)
	if args[0] == "digest" {
		since, until = today.AddDate(0, 0, -1), today
	}
	for _, f := range []struct {
		value string
		dst   *time.Time
	}{{*sinceFlag, &since}, {*untilFlag, &until}} {
		if f.value == "" {
			continue
		}
		t, err := time.ParseInLocation("2006-01-02", f.value, now.Location())
		if err != nil {
			fmt.Fprintf(os.Stderr, "sessions: bad date %q\n", f.value)
			return 2
		}
		*f.dst = t
	}
	sessions, err := LoadSessions(store, since, until, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sessions:", err)
		return 1
	}
	switch args[0] {
	case "digest":
		WriteDigest(os.Stdout, sessions, since, until)
		return 0
	case "search":
		if fs.NArg() == 0 {
			return usage()
		}
		if Search(os.Stdout, sessions, fs.Args()) == 0 {
			return 1
		}
		return 0
	}
	return usage()
}
