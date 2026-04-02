package sysutil

import (
	"testing"
)

func TestParseAWSProfiles(t *testing.T) {
	content := `[profile developer-access-767828768904]
sso_session = BlueWaterAutonomy
sso_account_id = 767828768904
sso_role_name = developer-access
region = us-east-2

[sso-session BlueWaterAutonomy]
sso_start_url = https://d-9067c20008.awsapps.com/start
sso_region = us-east-1

[default]
region = us-east-1
`
	profiles := ParseAWSProfiles(content)
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d: %v", len(profiles), profiles)
	}
	if profiles[0] != "developer-access-767828768904" {
		t.Errorf("expected developer-access-767828768904, got %s", profiles[0])
	}
	if profiles[1] != "default" {
		t.Errorf("expected default, got %s", profiles[1])
	}
}

func TestParseAWSProfilesEmpty(t *testing.T) {
	profiles := ParseAWSProfiles("")
	if len(profiles) != 0 {
		t.Errorf("expected 0 profiles, got %d", len(profiles))
	}
}

func TestParseAWSProfilesSkipsSSOSessions(t *testing.T) {
	content := `[sso-session MySession]
sso_start_url = https://example.com
`
	profiles := ParseAWSProfiles(content)
	if len(profiles) != 0 {
		t.Errorf("expected 0 profiles, got %d", len(profiles))
	}
}
