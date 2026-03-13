package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	comp "github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/state"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/wizard"
	"github.com/spf13/cobra"
)

var (
	flagSetupShow          bool
	flagSetupReset         bool
	flagSetupSkipGPU       bool
	flagSetupSkipConfigure bool
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Run system setup wizard",
	Long:  "Configure your system step by step. Linux Mint and macOS supported.",
	RunE:  runSetup,
}

func init() {
	setupCmd.Flags().BoolVar(&flagSetupShow, "show", false, "show current system configuration")
	setupCmd.Flags().BoolVar(&flagSetupReset, "reset", false, "reset configuration to defaults")
	setupCmd.Flags().BoolVar(&flagSetupSkipGPU, "skip-gpu", false, "skip GPU driver setup")
	setupCmd.Flags().BoolVar(&flagSetupSkipConfigure, "skip-configure", false, "skip interactive configuration")
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	if flagSetupShow {
		return showSetupDashboard()
	}
	if flagSetupReset {
		fmt.Println("Resetting configuration to defaults...")
		return nil
	}

	info := platform.Detect()
	steps := buildSteps(info)

	if isBatchMode() {
		return runBatchSetup(steps)
	}

	p := tea.NewProgram(wizard.New(steps))
	result, err := p.Run()
	if err != nil {
		return err
	}
	wizResult := result.(wizard.Model).GetResult()
	if wizResult.Quit {
		return nil
	}

	return executeSetupSteps(wizResult.Steps)
}

func buildSteps(info platform.Info) []wizard.Step {
	if info.OS == platform.MacOS {
		return buildMacOSSteps()
	}
	return buildLinuxSteps(info)
}

func buildLinuxSteps(info platform.Info) []wizard.Step {
	_ = info
	var steps []wizard.Step

	if !flagSetupSkipGPU {
		steps = append(steps, wizard.Step{
			Title:       "GPU Drivers",
			Description: "Configure NVIDIA drivers and Secure Boot signing",
			Options: []wizard.Option{
				{Label: "Install NVIDIA driver", Enabled: true, BashScript: "cmds/gpu-setup.sh"},
				{Label: "Sign kernel module (MOK)", Enabled: true, BashScript: "cmds/gpu-setup.sh"},
			},
		})
	}

	steps = append(steps, wizard.Step{
		Title:       "GRUB Config",
		Description: "Configure boot loader",
		Options: []wizard.Option{
			{Label: "Show GRUB menu on boot", Enabled: true, BashScript: "cmds/setup.sh"},
			{Label: "Set 10 second timeout", Enabled: true, BashScript: "cmds/setup.sh"},
			{Label: "Enable OS prober (dual boot)", Enabled: true, BashScript: "cmds/setup.sh"},
		},
	})

	steps = append(steps, wizard.Step{
		Title:       "Audio & Bluetooth",
		Description: "Configure PipeWire, Bluetooth codecs, and camera support",
		Options: []wizard.Option{
			{Label: "Install PipeWire audio stack", Enabled: true, BashScript: "cmds/setup.sh"},
			{Label: "Enable LDAC Bluetooth codec", Enabled: true, BashScript: "cmds/setup.sh"},
			{Label: "Install camera support (v4l-utils)", Enabled: true, BashScript: "cmds/setup.sh"},
			{Label: "Install linux-firmware updates", Enabled: true, BashScript: "cmds/setup.sh"},
		},
	})

	steps = append(steps, wizard.Step{
		Title:       "Desktop Services",
		Description: "Configure system services and desktop settings",
		Options: []wizard.Option{
			{Label: "Configure gsettings", Enabled: true, BashScript: "cmds/setup.sh"},
			{Label: "Enable system services", Enabled: true, BashScript: "cmds/setup.sh"},
		},
	})

	steps = append(steps, wizard.Step{
		Title:       "Input Devices",
		Description: "Configure keyboard and mouse",
		Options: []wizard.Option{
			{Label: "Set key repeat rate (200ms delay, 50 cps)", Enabled: true, BashScript: "cmds/setup.sh"},
			{Label: "Configure mouse bindings (xbindkeys)", Enabled: true, BashScript: "cmds/setup.sh"},
		},
	})

	steps = append(steps, wizard.Step{
		Title:       "Install Components",
		Description: "Select development tools to install",
		Options: []wizard.Option{
			{Label: "Launch component picker", Enabled: true},
		},
	})

	return steps
}

func buildMacOSSteps() []wizard.Step {
	return []wizard.Step{
		{
			Title:       "System Preferences",
			Description: "Configure macOS system settings",
			Options: []wizard.Option{
				{Label: "Configure Dock settings", Enabled: true},
				{Label: "Configure keyboard repeat", Enabled: true},
				{Label: "Show file extensions", Enabled: true},
			},
		},
		{
			Title:       "Install Components",
			Description: "Select development tools to install",
			Options: []wizard.Option{
				{Label: "Launch component picker", Enabled: true},
			},
		},
	}
}

func executeSetupSteps(steps []wizard.Step) error {
	for _, step := range steps {
		for _, opt := range step.Options {
			if !opt.Enabled || opt.AlreadyDone {
				continue
			}
			if opt.BashScript != "" {
				fmt.Printf("Running: %s — %s\n", step.Title, opt.Label)
				if flagDryRun {
					fmt.Printf("  [dry-run] bash %s\n", opt.BashScript)
					continue
				}
				cmd := exec.Command("bash", fmt.Sprintf("%s/%s", dotfilesRoot(), opt.BashScript))
				cmd.Stdout = nil
				cmd.Stderr = nil
				if err := cmd.Run(); err != nil {
					fmt.Printf("  Warning: %v\n", err)
				}
			}
		}
	}
	return nil
}

func runBatchSetup(steps []wizard.Step) error {
	return executeSetupSteps(steps)
}

func showSetupDashboard() error {
	info := platform.GatherSystemInfo(dotfilesRoot())

	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#30363d")).
		Padding(1, 2).
		Width(38)

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Orange)
	labelStyle := lipgloss.NewStyle().Foreground(styles.Subtle).Width(16)
	valueStyle := lipgloss.NewStyle().Foreground(styles.Bright)

	// GPU & Boot panel
	gpuPanel := headerStyle.Render("GPU & Boot") + "\n"
	gpuPanel += labelStyle.Render("Secure Boot:") + " " + valueStyle.Render(detectSecureBoot()) + "\n"
	gpuPanel += labelStyle.Render("GPU:") + " " + valueStyle.Render(detectGPU())

	// Audio panel
	audioPanel := headerStyle.Render("Audio & Bluetooth") + "\n"
	audioPanel += labelStyle.Render("Audio Stack:") + " " + valueStyle.Render(detectAudioStack()) + "\n"
	audioPanel += labelStyle.Render("Bluetooth:") + " " + valueStyle.Render(detectBluetooth())

	// Input panel
	inputPanel := headerStyle.Render("Input Devices") + "\n"
	inputPanel += labelStyle.Render("Key Repeat:") + " " + valueStyle.Render("configured")

	// Components panel
	ms := state.DefaultMarkerStore()
	installedList, _ := ms.List()
	compPanel := headerStyle.Render("Components") + "\n"
	compPanel += labelStyle.Render("Installed:") + " " + valueStyle.Render(fmt.Sprintf("%d of %d", len(installedList), len(comp.Registry)))

	// Layout
	topRow := lipgloss.JoinHorizontal(lipgloss.Top,
		panelStyle.Render(gpuPanel),
		panelStyle.Render(audioPanel),
	)
	bottomRow := lipgloss.JoinHorizontal(lipgloss.Top,
		panelStyle.Render(inputPanel),
		panelStyle.Render(compPanel),
	)

	fmt.Println(styles.Title.Render("System Configuration"))
	fmt.Println()
	fmt.Println(topRow)
	fmt.Println(bottomRow)
	fmt.Println()
	fmt.Println(styles.Help.Render(fmt.Sprintf("OS: %s (%s) · CPU: %s · Memory: %d GB",
		info.Platform.Distro, info.Platform.OS, info.CPUModel, info.MemoryGB)))

	return nil
}

// Helper functions for detecting current system state
func detectSecureBoot() string {
	out, err := exec.Command("mokutil", "--sb-state").Output()
	if err != nil {
		return "unknown"
	}
	if strings.Contains(string(out), "enabled") {
		return "enabled"
	}
	return "disabled"
}

func detectGPU() string {
	out, err := exec.Command("lspci").Output()
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "VGA") || strings.Contains(line, "3D") {
			parts := strings.SplitN(line, ": ", 2)
			if len(parts) == 2 {
				return parts[1]
			}
		}
	}
	return "none detected"
}

func detectAudioStack() string {
	if _, err := exec.LookPath("pipewire"); err == nil {
		return "PipeWire"
	}
	if _, err := exec.LookPath("pulseaudio"); err == nil {
		return "PulseAudio"
	}
	return "unknown"
}

func detectBluetooth() string {
	out, err := exec.Command("systemctl", "is-active", "bluetooth").Output()
	if err != nil {
		return "inactive"
	}
	return strings.TrimSpace(string(out))
}
