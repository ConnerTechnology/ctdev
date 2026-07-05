package setup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// defaultsWrite runs `defaults write <domain> <key> <args...>` and appends any
// failure to errs. A failure is logged to w but doesn't abort — subsequent
// writes still run so unrelated keys apply even if one is SIP-restricted.
func defaultsWrite(ctx context.Context, w io.Writer, errs *[]error, domain, key string, args ...string) {
	full := append([]string{"write", domain, key}, args...)
	if err := exec.CommandContext(ctx, "defaults", full...).Run(); err != nil {
		fmt.Fprintf(w, "warning: defaults write %s %s failed: %v\n", domain, key, err)
		*errs = append(*errs, fmt.Errorf("defaults write %s %s: %w", domain, key, err))
	}
}

// detectMacOSDefaults reports whether the ctdev macOS defaults look applied,
// checking Dock auto-hide as a representative key.
func detectMacOSDefaults(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "defaults", "read", "com.apple.dock", "autohide").Output()
	if err != nil {
		return "not applied"
	}
	if strings.TrimSpace(string(out)) == "1" {
		return "applied"
	}
	return "not applied"
}

// ApplyMacOSDefaults writes the opinionated `defaults` settings for Dock,
// Finder, keyboard, dialogs, and screensaver security, then restarts Dock and
// Finder. Exposed as the `macos` configure category.
func ApplyMacOSDefaults(ctx context.Context, o sysutil.Opts) error {
	w := o.Stdout
	if o.DryRun {
		fmt.Fprintln(w, "[dry-run] Would configure Dock settings (auto-hide, animations, recent apps)")
		fmt.Fprintln(w, "[dry-run] Would configure Sound settings (volume change feedback)")
		fmt.Fprintln(w, "[dry-run] Would configure Finder settings (path bar, status bar)")
		fmt.Fprintln(w, "[dry-run] Would configure Keyboard settings (disable smart quotes/dashes)")
		fmt.Fprintln(w, "[dry-run] Would configure Dialog settings (expand save/print dialogs)")
		fmt.Fprintln(w, "[dry-run] Would configure Security settings (require password after sleep)")
		fmt.Fprintln(w, "[dry-run] Would restart Dock and Finder")
		return nil
	}

	var errs []error

	// Dock
	fmt.Fprintln(w, "Configuring Dock...")
	defaultsWrite(ctx, w, &errs, "com.apple.dock", "autohide", "-bool", "true")
	defaultsWrite(ctx, w, &errs, "com.apple.dock", "launchanim", "-bool", "false")
	defaultsWrite(ctx, w, &errs, "com.apple.dock", "show-recents", "-bool", "false")

	// Sound
	fmt.Fprintln(w, "Configuring Sound...")
	defaultsWrite(ctx, w, &errs, "NSGlobalDomain", "com.apple.sound.beep.feedback", "-bool", "true")

	// Finder
	fmt.Fprintln(w, "Configuring Finder...")
	defaultsWrite(ctx, w, &errs, "com.apple.finder", "ShowPathbar", "-bool", "true")
	defaultsWrite(ctx, w, &errs, "com.apple.finder", "ShowStatusBar", "-bool", "true")
	defaultsWrite(ctx, w, &errs, "com.apple.desktopservices", "DSDontWriteNetworkStores", "-bool", "true")
	defaultsWrite(ctx, w, &errs, "com.apple.desktopservices", "DSDontWriteUSBStores", "-bool", "true")
	defaultsWrite(ctx, w, &errs, "com.apple.finder", "FXDefaultSearchScope", "-string", "SCcf")
	defaultsWrite(ctx, w, &errs, "com.apple.finder", "FXPreferredViewStyle", "-string", "Nlsv")
	defaultsWrite(ctx, w, &errs, "com.apple.finder", "QuitMenuItem", "-bool", "true")

	// Keyboard
	fmt.Fprintln(w, "Configuring Keyboard...")
	defaultsWrite(ctx, w, &errs, "NSGlobalDomain", "NSAutomaticQuoteSubstitutionEnabled", "-bool", "false")
	defaultsWrite(ctx, w, &errs, "NSGlobalDomain", "NSAutomaticDashSubstitutionEnabled", "-bool", "false")
	defaultsWrite(ctx, w, &errs, "NSGlobalDomain", "NSAutomaticSpellingCorrectionEnabled", "-bool", "false")
	defaultsWrite(ctx, w, &errs, "NSGlobalDomain", "NSAutomaticCapitalizationEnabled", "-bool", "false")
	defaultsWrite(ctx, w, &errs, "NSGlobalDomain", "NSAutomaticPeriodSubstitutionEnabled", "-bool", "false")
	defaultsWrite(ctx, w, &errs, "NSGlobalDomain", "KeyRepeat", "-int", "2")
	defaultsWrite(ctx, w, &errs, "NSGlobalDomain", "InitialKeyRepeat", "-int", "15")

	// Dialogs
	fmt.Fprintln(w, "Configuring Dialogs...")
	defaultsWrite(ctx, w, &errs, "NSGlobalDomain", "NSNavPanelExpandedStateForSaveMode", "-bool", "true")
	defaultsWrite(ctx, w, &errs, "NSGlobalDomain", "NSNavPanelExpandedStateForSaveMode2", "-bool", "true")
	defaultsWrite(ctx, w, &errs, "NSGlobalDomain", "PMPrintingExpandedStateForPrint", "-bool", "true")
	defaultsWrite(ctx, w, &errs, "NSGlobalDomain", "PMPrintingExpandedStateForPrint2", "-bool", "true")

	// Security
	fmt.Fprintln(w, "Configuring Security...")
	defaultsWrite(ctx, w, &errs, "com.apple.screensaver", "askForPassword", "-int", "1")
	defaultsWrite(ctx, w, &errs, "com.apple.screensaver", "askForPasswordDelay", "-int", "0")

	// Apply
	fmt.Fprintln(w, "Applying changes...")
	_ = exec.CommandContext(ctx, "killall", "Dock").Run()
	_ = exec.CommandContext(ctx, "killall", "Finder").Run()

	if len(errs) > 0 {
		fmt.Fprintf(w, "macOS defaults applied with %d warning(s)\n", len(errs))
		return errors.Join(errs...)
	}
	fmt.Fprintln(w, "macOS defaults configured")
	return nil
}
