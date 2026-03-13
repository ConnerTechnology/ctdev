package info

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
)

type DiskInfo struct {
	Mount   string
	Total   string
	Used    string
	Percent int
}

func Render(sysInfo platform.SystemInfo, version string, installedCount, totalCount int, installedNames []string) string {
	var b strings.Builder

	b.WriteString(styles.Title.Render("System Information"))
	b.WriteString("\n\n")

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Orange)
	labelStyle := lipgloss.NewStyle().Foreground(styles.Subtle).Width(20)
	valueStyle := lipgloss.NewStyle().Foreground(styles.Bright)

	// System section
	b.WriteString(headerStyle.Render("System"))
	b.WriteString("\n")
	osStr := string(sysInfo.Platform.OS)
	if sysInfo.Platform.Distro != "" {
		osStr = fmt.Sprintf("%s (%s)", sysInfo.Platform.Distro, sysInfo.Platform.OS)
	}
	b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("OS"), valueStyle.Render(osStr)))
	b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Architecture"), valueStyle.Render(sysInfo.Platform.Arch)))
	b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Package Manager"), valueStyle.Render(sysInfo.Platform.PackageManager)))
	b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Shell"), valueStyle.Render(sysInfo.Shell)))
	b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Dotfiles"), valueStyle.Render(sysInfo.DotfilesDir)))
	b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("ctdev"), valueStyle.Render(version)))

	// Hardware section
	b.WriteString("\n")
	b.WriteString(headerStyle.Render("Hardware"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s %s (%d threads)\n", labelStyle.Render("CPU"), valueStyle.Render(sysInfo.CPUModel), sysInfo.CPUThreads))
	b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Memory"), valueStyle.Render(fmt.Sprintf("%d GB", sysInfo.MemoryGB))))

	// Disk section
	b.WriteString("\n")
	b.WriteString(headerStyle.Render("Disk"))
	b.WriteString("\n")
	disks := getDiskInfo()
	for _, d := range disks {
		bar := renderDiskBar(d.Percent, 30)
		b.WriteString(fmt.Sprintf("  %s %s %s\n", labelStyle.Render(d.Mount), bar, styles.Dimmed.Render(fmt.Sprintf("%s / %s (%d%%)", d.Used, d.Total, d.Percent))))
	}

	// Components section
	b.WriteString("\n")
	b.WriteString(headerStyle.Render("Components"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %d of %d installed\n", installedCount, totalCount))
	if len(installedNames) > 0 {
		pillStyle := lipgloss.NewStyle().
			Foreground(styles.Blue).
			Background(lipgloss.Color("#1f6feb33")).
			Padding(0, 1)
		b.WriteString("  ")
		for i, name := range installedNames {
			if i > 0 {
				b.WriteString(" ")
			}
			b.WriteString(pillStyle.Render(name))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func getDiskInfo() []DiskInfo {
	out, err := exec.Command("df", "-h", "--output=target,size,used,pcent").Output()
	if err != nil {
		return nil
	}
	var disks []DiskInfo
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 4 {
			continue
		}
		mount := fields[0]
		if mount != "/" && mount != "/home" {
			continue
		}
		pctStr := strings.TrimRight(fields[3], "%")
		pct, _ := strconv.Atoi(pctStr)
		disks = append(disks, DiskInfo{
			Mount:   mount,
			Total:   fields[1],
			Used:    fields[2],
			Percent: pct,
		})
	}
	return disks
}

func renderDiskBar(percent, width int) string {
	filled := width * percent / 100
	if filled > width {
		filled = width
	}
	empty := width - filled

	color := styles.Green
	if percent > 85 {
		color = styles.Red
	} else if percent > 60 {
		color = styles.Yellow
	}

	bar := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("█", filled))
	bar += lipgloss.NewStyle().Foreground(styles.Subtle).Render(strings.Repeat("░", empty))
	return bar
}
