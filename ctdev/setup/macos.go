package setup

import (
	"fmt"
	"io"
	"os/exec"
)

func ApplyMacOSDefaults(w io.Writer, dryRun bool) error {
	if dryRun {
		fmt.Fprintln(w, "[dry-run] Would configure Dock settings (auto-hide, animations, recent apps)")
		fmt.Fprintln(w, "[dry-run] Would configure Sound settings (volume change feedback)")
		fmt.Fprintln(w, "[dry-run] Would configure Finder settings (path bar, status bar)")
		fmt.Fprintln(w, "[dry-run] Would configure Keyboard settings (disable smart quotes/dashes)")
		fmt.Fprintln(w, "[dry-run] Would configure Dialog settings (expand save/print dialogs)")
		fmt.Fprintln(w, "[dry-run] Would configure Security settings (require password after sleep)")
		fmt.Fprintln(w, "[dry-run] Would restart Dock and Finder")
		return nil
	}

	// Dock
	fmt.Fprintln(w, "Configuring Dock...")
	run("defaults", "write", "com.apple.dock", "autohide", "-bool", "true")
	run("defaults", "write", "com.apple.dock", "launchanim", "-bool", "false")
	run("defaults", "write", "com.apple.dock", "show-recents", "-bool", "false")

	// Sound
	fmt.Fprintln(w, "Configuring Sound...")
	run("defaults", "write", "NSGlobalDomain", "com.apple.sound.beep.feedback", "-bool", "true")

	// Finder
	fmt.Fprintln(w, "Configuring Finder...")
	run("defaults", "write", "com.apple.finder", "ShowPathbar", "-bool", "true")
	run("defaults", "write", "com.apple.finder", "ShowStatusBar", "-bool", "true")
	run("defaults", "write", "com.apple.desktopservices", "DSDontWriteNetworkStores", "-bool", "true")
	run("defaults", "write", "com.apple.desktopservices", "DSDontWriteUSBStores", "-bool", "true")
	run("defaults", "write", "com.apple.finder", "FXDefaultSearchScope", "-string", "SCcf")
	run("defaults", "write", "com.apple.finder", "FXPreferredViewStyle", "-string", "Nlsv")
	run("defaults", "write", "com.apple.finder", "QuitMenuItem", "-bool", "true")

	// Keyboard
	fmt.Fprintln(w, "Configuring Keyboard...")
	run("defaults", "write", "NSGlobalDomain", "NSAutomaticQuoteSubstitutionEnabled", "-bool", "false")
	run("defaults", "write", "NSGlobalDomain", "NSAutomaticDashSubstitutionEnabled", "-bool", "false")
	run("defaults", "write", "NSGlobalDomain", "NSAutomaticSpellingCorrectionEnabled", "-bool", "false")
	run("defaults", "write", "NSGlobalDomain", "NSAutomaticCapitalizationEnabled", "-bool", "false")
	run("defaults", "write", "NSGlobalDomain", "NSAutomaticPeriodSubstitutionEnabled", "-bool", "false")
	run("defaults", "write", "NSGlobalDomain", "KeyRepeat", "-int", "2")
	run("defaults", "write", "NSGlobalDomain", "InitialKeyRepeat", "-int", "15")

	// Dialogs
	fmt.Fprintln(w, "Configuring Dialogs...")
	run("defaults", "write", "NSGlobalDomain", "NSNavPanelExpandedStateForSaveMode", "-bool", "true")
	run("defaults", "write", "NSGlobalDomain", "NSNavPanelExpandedStateForSaveMode2", "-bool", "true")
	run("defaults", "write", "NSGlobalDomain", "PMPrintingExpandedStateForPrint", "-bool", "true")
	run("defaults", "write", "NSGlobalDomain", "PMPrintingExpandedStateForPrint2", "-bool", "true")

	// Security
	fmt.Fprintln(w, "Configuring Security...")
	run("defaults", "write", "com.apple.screensaver", "askForPassword", "-int", "1")
	run("defaults", "write", "com.apple.screensaver", "askForPasswordDelay", "-int", "0")

	// Apply
	fmt.Fprintln(w, "Applying changes...")
	_ = exec.Command("killall", "Dock").Run()
	_ = exec.Command("killall", "Finder").Run()

	fmt.Fprintln(w, "macOS defaults configured")
	return nil
}

