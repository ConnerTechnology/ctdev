package cmd

import (
	"strings"
	"testing"

	"github.com/ConnerTechnology/dotfiles/ctdev/profile"
)

func TestValidateProfile_BuiltinsAreValid(t *testing.T) {
	// Every shipped profile must reference only real components and categories —
	// this is the guard that keeps profiles from rotting as the registry evolves.
	for _, p := range profile.List() {
		p := p
		if err := validateProfile(&p); err != nil {
			t.Errorf("built-in profile %s invalid: %v", p.Name, err)
		}
	}
}

func TestValidateProfile_RejectsUnknownAndGPU(t *testing.T) {
	p := &profile.Profile{
		Name:       "bad",
		Components: []string{"zsh", "not-a-component"},
		Configure:  []string{"ssh", "not-a-category", "gpu"},
	}
	err := validateProfile(p)
	if err == nil {
		t.Fatal("expected validation error")
	}
	for _, want := range []string{"not-a-component", "not-a-category", "gpu"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}
