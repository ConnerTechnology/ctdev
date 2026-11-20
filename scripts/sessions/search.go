package main

import (
	"fmt"
	"io"
	"strings"
)

const (
	maxHits     = 20
	snippetSide = 100
)

// Search prints where every term appears (case-insensitive) in one prompt, reply, tool call or
// tool output, newest session first. Skill bodies aren't searched.
func Search(w io.Writer, sessions []*Session, terms []string) int {
	lower := make([]string, len(terms))
	for i, t := range terms {
		lower[i] = strings.ToLower(t)
	}
	terms = lower
	hits := 0
	for i := len(sessions) - 1; i >= 0 && hits < maxHits; i-- {
		s := sessions[i]
		for _, e := range s.entries {
			// A skill's body is the skill's text, not the session's, loaded whole each time it is
			// used; its copies would fill the hits with nothing the session said or did.
			if e.Skill() != "" {
				continue
			}
			for _, b := range e.Content {
				for _, text := range b.texts() {
					if hits >= maxHits || !containsAll(strings.ToLower(text), terms) {
						continue
					}
					hits++
					where := e.Type
					if b.Name != "" {
						where = b.Name
					}
					fmt.Fprintf(w, "%s %s %s (%s, checkpoint %s)\n  %s\n", short(s.SessionID),
						e.TS.Local().Format("2006-01-02 15:04"), where, s.Branch, s.CheckpointID, snippet(text, terms[0]))
				}
			}
		}
	}
	if hits == 0 {
		fmt.Fprintln(w, "No matches.")
	}
	return hits
}

func (inst Block) texts() []string {
	out := []string{inst.Text}
	if t := inst.Target(); t != "" {
		out = append(out, t)
	}
	if inst.Result != nil {
		out = append(out, inst.Result.Output)
	}
	return out
}

func containsAll(text string, terms []string) bool {
	if text == "" {
		return false
	}
	for _, t := range terms {
		if !strings.Contains(text, t) {
			return false
		}
	}
	return true
}

// snippet is the text around the first match, in runes so a character is never cut in half.
func snippet(text, term string) string {
	r := []rune(text)
	t := []rune(term)
	i := 0
	for j := 0; j+len(t) <= len(r); j++ {
		if strings.EqualFold(string(r[j:j+len(t)]), term) {
			i = j
			break
		}
	}
	lo, hi := max(0, i-snippetSide), min(len(r), i+len(t)+snippetSide)
	out := clip(string(r[lo:hi]), 2*snippetSide+len(t))
	if lo > 0 {
		out = "..." + out
	}
	return out
}
