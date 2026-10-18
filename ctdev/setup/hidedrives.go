package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// Hide drives (`desktop`): a udev rule that sets UDISKS_IGNORE on the
// partitions the user checks, keyed by filesystem UUID so it hides the same
// partitions whatever the kernel names the disk on a given boot.

// hideDrivesRulePath is the rule ctdev owns. A var so tests use a temp dir.
var hideDrivesRulePath = "/etc/udev/rules.d/99-hide-drives.rules"

// hideDrivesNone is the value when no partition is hidden (no rule file).
const hideDrivesNone = PicksNone

// hideDrivesLegacy is the value detect reports for a rule ctdev didn't write
// in this form, such as the device-name rule it embedded before CTD-94. Any
// choice rewrites it.
const hideDrivesLegacy = "legacy rule"

// hideDrivesUnknown is the value detect reports when the rule can't be read.
const hideDrivesUnknown = "unknown"

// lsblkRead lists block devices as JSON. Read-only; tests substitute a fake.
var lsblkRead = func(ctx context.Context) ([]byte, error) {
	return exec.CommandContext(ctx, "lsblk", "-J", "-l", "-o", "NAME,UUID,FSTYPE,LABEL,SIZE,MOUNTPOINT,TYPE").Output()
}

// hideDrivesWrite and hideDrivesRun change the system as root; tests
// substitute fakes.
var (
	hideDrivesWrite = sysutil.SudoWriteFile
	hideDrivesRun   = sysutil.SudoRun
)

// fsUUID matches the filesystem UUIDs lsblk reports (ext4's dashed UUID,
// vfat's XXXX-XXXX, NTFS's 16 hex digits) and nothing that could break out of
// a quoted udev match.
var fsUUID = regexp.MustCompile(`^[0-9A-Za-z-]+$`)

// hideDrivesRuleLine matches one line of the rule ctdev writes.
var hideDrivesRuleLine = regexp.MustCompile(`^ENV\{ID_FS_UUID\}=="([0-9A-Za-z-]+)", ENV\{UDISKS_IGNORE\}="1"$`)

type lsblkDevice struct {
	Name       string  `json:"name"`
	UUID       *string `json:"uuid"`
	FSType     *string `json:"fstype"`
	Label      *string `json:"label"`
	Size       string  `json:"size"`
	Mountpoint *string `json:"mountpoint"`
	Type       string  `json:"type"`
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// hideDrivesChoices lists this machine's partitions that have a filesystem
// UUID, in lsblk order. A partition without one can't be keyed, so it isn't
// offered.
//
// Partitions the rule hides now that aren't attached are added at the end as
// "(not connected)", so re-picking keeps them hidden unless unchecked.
func hideDrivesChoices(ctx context.Context) []PickerChoice {
	choices := attachedPartitions(ctx)
	listed := make(map[string]bool)
	for _, c := range choices {
		listed[c.Value] = true
	}
	for _, u := range SplitPicks(detectHideDrives(ctx)) {
		if !listed[u] && fsUUID.MatchString(u) {
			choices = append(choices, PickerChoice{Value: u, Description: "hidden now (not connected)"})
		}
	}
	return choices
}

// attachedPartitions lists the partitions lsblk sees that have a UUID.
func attachedPartitions(ctx context.Context) []PickerChoice {
	out, err := lsblkRead(ctx)
	if err != nil {
		return nil
	}
	var parsed struct {
		BlockDevices []lsblkDevice `json:"blockdevices"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil
	}
	var choices []PickerChoice
	for _, d := range parsed.BlockDevices {
		uuid := deref(d.UUID)
		if d.Type != "part" || uuid == "" || !fsUUID.MatchString(uuid) {
			continue
		}
		parts := []string{"/dev/" + d.Name}
		if fs := deref(d.FSType); fs != "" {
			parts = append(parts, fs)
		}
		if d.Size != "" {
			parts = append(parts, d.Size)
		}
		if l := deref(d.Label); l != "" {
			parts = append(parts, `"`+l+`"`)
		}
		if m := deref(d.Mountpoint); m != "" {
			parts = append(parts, "mounted at "+m)
		}
		choices = append(choices, PickerChoice{Value: uuid, Description: strings.Join(parts, " · ")})
	}
	return choices
}

// renderHideDrivesRule returns the rule file hiding exactly uuids.
func renderHideDrivesRule(uuids []string) string {
	var b strings.Builder
	b.WriteString("# Written by `ctdev configure desktop` (Hide drives). Edit there, not here.\n")
	b.WriteString("# Hides these partitions, by filesystem UUID, from the file manager.\n")
	for _, u := range uuids {
		fmt.Fprintf(&b, "ENV{ID_FS_UUID}==%q, ENV{UDISKS_IGNORE}=\"1\"\n", u)
	}
	return b.String()
}

// detectHideDrives returns the hidden UUIDs as a pick value, hideDrivesNone
// when there is no rule, or hideDrivesLegacy for a rule in another form.
func detectHideDrives(_ context.Context) string {
	content, err := os.ReadFile(hideDrivesRulePath)
	if errors.Is(err, os.ErrNotExist) {
		return hideDrivesNone
	}
	if err != nil {
		return hideDrivesUnknown
	}
	var uuids []string
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := hideDrivesRuleLine.FindStringSubmatch(line)
		if m == nil {
			return hideDrivesLegacy
		}
		uuids = append(uuids, m[1])
	}
	return JoinPicks(uuids)
}

// applyHideDrives writes the rule for the UUIDs in value, or removes it when
// value checks nothing, then reloads udev and re-triggers block devices so
// the file manager sees the change without a reboot.
func applyHideDrives(ctx context.Context, o sysutil.Opts, value string) error {
	if value == hideDrivesLegacy || value == hideDrivesUnknown {
		// Detect's reports, not a choice: leave the rule as it is.
		return nil
	}
	uuids := SplitPicks(value)
	for _, u := range uuids {
		if !fsUUID.MatchString(u) {
			return fmt.Errorf("hide drives: %q is not a filesystem UUID", u)
		}
	}
	if len(uuids) == 0 {
		if _, err := os.Stat(hideDrivesRulePath); errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err := hideDrivesRun(ctx, o, "rm", "-f", hideDrivesRulePath); err != nil {
			return fmt.Errorf("remove hide-drives udev rule: %w", err)
		}
	} else if err := hideDrivesWrite(ctx, o, renderHideDrivesRule(uuids), hideDrivesRulePath); err != nil {
		return fmt.Errorf("write hide-drives udev rule: %w", err)
	}
	if err := hideDrivesRun(ctx, o, "udevadm", "control", "--reload-rules"); err != nil {
		return fmt.Errorf("reload udev rules: %w", err)
	}
	if err := hideDrivesRun(ctx, o, "udevadm", "trigger", "--subsystem-match=block", "--action=change"); err != nil {
		return fmt.Errorf("re-trigger block devices: %w", err)
	}
	return nil
}
