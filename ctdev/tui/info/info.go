package info

import (
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
)

type DiskInfo struct {
	Mount   string
	TotalKB int64
	UsedKB  int64
	Percent int
}

type ComponentInfo struct {
	Name      string
	Category  string
	Installed bool
}

// ProfileInfo names the machine profile this host was built from, and how much
// of it is actually present. Inferred is set when no `ctdev apply` recorded a
// profile and the name is a best guess from what's installed.
type ProfileInfo struct {
	Name      string
	Inferred  bool
	Installed int
	Total     int
}

func Render(sysInfo platform.SystemInfo, version string, components []ComponentInfo, prof *ProfileInfo, termWidth int) string {
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
		distro := sysInfo.Platform.Distro
		if v := sysInfo.Platform.DistroVersion; v != "" {
			distro += " " + v
		}
		if c := sysInfo.Platform.Codename; c != "" {
			distro += " (" + c + ")"
		}
		osStr = fmt.Sprintf("%s (%s)", distro, sysInfo.Platform.OS)
	}
	b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("OS"), valueStyle.Render(osStr)))
	if sysInfo.Kernel != "" {
		b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Kernel"), valueStyle.Render(sysInfo.Kernel)))
	}
	b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Architecture"), valueStyle.Render(sysInfo.Platform.Arch)))
	b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Package Manager"), valueStyle.Render(sysInfo.Platform.PackageManager)))
	b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Shell"), valueStyle.Render(sysInfo.Shell)))
	b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Dotfiles"), valueStyle.Render(sysInfo.DotfilesDir)))
	b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("ctdev"), valueStyle.Render(version)))
	if sysInfo.Uptime > 0 {
		b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Uptime"), valueStyle.Render(humanUptime(sysInfo.Uptime))))
	}
	if prof != nil {
		b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Profile"), renderProfile(prof)))
	}

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

	// Usage section — live consumption, as opposed to the Hardware specs above.
	b.WriteString("\n")
	b.WriteString(headerStyle.Render("Usage"))
	b.WriteString("\n")
	if sysInfo.MemTotalKB > 0 {
		pct := int(sysInfo.MemUsedKB * 100 / sysInfo.MemTotalKB)
		b.WriteString(fmt.Sprintf("  %s %s %s\n", labelStyle.Render("Memory"), renderDiskBar(pct, 30),
			usageDetail(sysInfo.MemUsedKB, sysInfo.MemTotalKB, pct)))
	}
	// Disks get their own indented group under a "Disk" heading: a bare "/"
	// column doesn't tell you it's a filesystem, and a machine with several
	// mounted volumes needs them visibly grouped rather than listed loose.
	if disks := getDiskInfo(); len(disks) > 0 {
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  %s\n", styles.Dimmed.Render("Disk")))
		// Indent by 2 more and narrow the label by 2, so the bars stay in the
		// same column as Memory's.
		diskLabel := styles.Label(18)
		for i, d := range disks {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(fmt.Sprintf("    %s %s %s\n", diskLabel.Render(diskLabelText(d.Mount)),
				renderDiskBar(d.Percent, 30), usageDetail(d.UsedKB, d.TotalKB, d.Percent)))
		}
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

// diskFloorKB hides filesystems too small to be worth a row — the EFI system
// partition and similar. /boot (typically ~1.7G) stays: it fills up with old
// kernels and that's worth seeing.
const diskFloorKB = 1024 * 1024 // 1 GiB

func getDiskInfo() []DiskInfo {
	out, err := exec.Command("df", "-Pk", "--local",
		"-x", "tmpfs", "-x", "devtmpfs", "-x", "squashfs", "-x", "overlay", "-x", "efivarfs").Output()
	if err != nil {
		return nil
	}
	return parseDiskInfo(string(out))
}

// parseDiskInfo turns `df -Pk` output into the rows to render: every real
// filesystem above the size floor, root first and the rest alphabetical so the
// list is stable between runs.
func parseDiskInfo(out string) []DiskInfo {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return nil
	}
	var disks []DiskInfo
	for _, line := range lines[1:] { // skip the header
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		totalKB, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || totalKB < diskFloorKB {
			continue
		}
		usedKB, _ := strconv.ParseInt(fields[2], 10, 64)
		pct, _ := strconv.Atoi(strings.TrimSuffix(fields[4], "%"))
		disks = append(disks, DiskInfo{Mount: fields[5], TotalKB: totalKB, UsedKB: usedKB, Percent: pct})
	}
	sort.Slice(disks, func(i, j int) bool {
		if (disks[i].Mount == "/") != (disks[j].Mount == "/") {
			return disks[i].Mount == "/"
		}
		return disks[i].Mount < disks[j].Mount
	})
	return disks
}

// usageDetail is the "used / total (pct)" tail shared by the memory and disk
// rows, so both read in the same units.
func usageDetail(usedKB, totalKB int64, pct int) string {
	return styles.Dimmed.Render(fmt.Sprintf("%s / %s (%d%%)", sysutil.HumanKB(usedKB), sysutil.HumanKB(totalKB), pct))
}

// diskLabelText annotates root, whose bare "/" says nothing about what it
// holds. Every other mount path (/boot, /mnt/Timeshift) already reads clearly.
func diskLabelText(mount string) string {
	if mount == "/" {
		return "/ (system)"
	}
	return mount
}

// renderProfile shows the profile and how complete it is, pointing at
// `ctdev diff` only when something is actually missing.
func renderProfile(p *ProfileInfo) string {
	out := styles.Value.Render(p.Name)
	if p.Inferred {
		out += styles.Dimmed.Render(" (closest match)")
	}
	missing := p.Total - p.Installed
	if missing <= 0 {
		return out + styles.Dimmed.Render(fmt.Sprintf(" · %d/%d components", p.Installed, p.Total))
	}
	return out + styles.Warning.Render(fmt.Sprintf(" · %d/%d components", p.Installed, p.Total)) +
		styles.Dimmed.Render(fmt.Sprintf(" — ctdev diff %s", p.Name))
}

// humanUptime renders uptime at the coarsest useful precision: minutes for a
// fresh boot, then hours, then days — an always-on box reads "34d", not "816h".
func humanUptime(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		days := int(d.Hours()) / 24
		if hours := int(d.Hours()) % 24; hours > 0 {
			return fmt.Sprintf("%dd %dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	}
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
