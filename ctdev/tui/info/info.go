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

type ComponentInfo struct {
	Name      string
	Category  string
	Installed bool
}

func Render(sysInfo platform.SystemInfo, version string, components []ComponentInfo, termWidth int) string {
	var b strings.Builder

	b.WriteString(styles.Title.Render("System Information"))
	b.WriteString("\n\n")

	headerStyle := styles.Header
	labelStyle := styles.Label(20)
	valueStyle := styles.Value

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
	for i, gpu := range sysInfo.GPUs {
		label := "GPU"
		if len(sysInfo.GPUs) > 1 {
			label = fmt.Sprintf("GPU %d", i+1)
		}
		val := gpu.Name
		if gpu.Driver != "" {
			val += styles.Dimmed.Render(fmt.Sprintf(" (%s)", gpu.Driver))
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render(label), val))
	}
	for _, net := range sysInfo.Network {
		label := "Network"
		if net.Type == "wifi" {
			label = "Wi-Fi"
		} else if net.Type == "ethernet" {
			label = "Ethernet"
		}
		val := net.Name
		if net.Interface != "" {
			val += styles.Dimmed.Render(fmt.Sprintf(" (%s)", net.Interface))
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render(label), val))
	}

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
	installedCount := 0
	for _, c := range components {
		if c.Installed {
			installedCount++
		}
	}
	b.WriteString(headerStyle.Render(fmt.Sprintf("Components (%d/%d)", installedCount, len(components))))
	b.WriteString("\n")

	// Group by category, preserving order
	type catGroup struct {
		name  string
		items []ComponentInfo
	}
	var groups []catGroup
	seen := make(map[string]int)
	for _, c := range components {
		if idx, ok := seen[c.Category]; ok {
			groups[idx].items = append(groups[idx].items, c)
		} else {
			seen[c.Category] = len(groups)
			groups = append(groups, catGroup{name: c.Category, items: []ComponentInfo{c}})
		}
	}

	colWidth := 20
	cols := 2
	if termWidth > 0 {
		if termWidth < 60 {
			cols = 1
		} else if termWidth > 100 {
			cols = 3
		}
	}

	for _, g := range groups {
		catInstalled := 0
		for _, c := range g.items {
			if c.Installed {
				catInstalled++
			}
		}
		b.WriteString(fmt.Sprintf("  %s %s\n",
			styles.Dimmed.Render(g.name),
			styles.Dimmed.Render(fmt.Sprintf("(%d/%d)", catInstalled, len(g.items))),
		))
		for i := 0; i < len(g.items); i += cols {
			b.WriteString("    ")
			for j := 0; j < cols && i+j < len(g.items); j++ {
				b.WriteString(renderComponentEntry(g.items[i+j], colWidth))
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

func renderComponentEntry(c ComponentInfo, width int) string {
	if c.Installed {
		icon := lipgloss.NewStyle().Foreground(styles.Green).Render("✓")
		name := lipgloss.NewStyle().Foreground(styles.Green).Width(width - 2).Render(c.Name)
		return icon + " " + name
	}
	icon := lipgloss.NewStyle().Foreground(styles.Subtle).Render("○")
	name := styles.Label(width - 2).Render(c.Name)
	return icon + " " + name
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
