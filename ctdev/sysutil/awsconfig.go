package sysutil

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	profileRegex = regexp.MustCompile(`^\[profile\s+(.+)\]$`)
	defaultRegex = regexp.MustCompile(`^\[default\]$`)
)

func ParseAWSProfiles(content string) []string {
	var profiles []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if m := profileRegex.FindStringSubmatch(line); len(m) == 2 {
			profiles = append(profiles, m[1])
		} else if defaultRegex.MatchString(line) {
			profiles = append(profiles, "default")
		}
	}
	return profiles
}

func ReadAWSProfiles() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(home, ".aws", "config"))
	if err != nil {
		return nil, err
	}
	return ParseAWSProfiles(string(data)), nil
}
