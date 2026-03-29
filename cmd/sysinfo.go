package cmd

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/docker"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/spf13/cobra"
)

// ANSI color codes — disabled on Windows
var (
	colorReset  string
	colorBold   string
	colorCyan   string
	colorGreen  string
	colorYellow string
	colorRed    string
	colorBlue   string
	colorGray   string
)

func initColors() {
	if runtime.GOOS == "windows" {
		return
	}
	colorReset = "\033[0m"
	colorBold = "\033[1m"
	colorCyan = "\033[36m"
	colorGreen = "\033[32m"
	colorYellow = "\033[33m"
	colorRed = "\033[31m"
	colorBlue = "\033[34m"
	colorGray = "\033[90m"
}

func header(title string) string {
	return fmt.Sprintf("%s%s%s %s%s%s",
		colorBold, colorCyan, "▶", title, colorReset, "")
}

func label(s string) string {
	return fmt.Sprintf("%s%-16s%s", colorGray, s, colorReset)
}

func usageBar(pct float64, width int) string {
	filled := int(math.Round(pct / 100.0 * float64(width)))
	if filled > width {
		filled = width
	}
	empty := width - filled

	var barColor string
	switch {
	case pct >= 85:
		barColor = colorRed
	case pct >= 60:
		barColor = colorYellow
	default:
		barColor = colorGreen
	}

	bar := fmt.Sprintf("%s%s%s%s%s",
		barColor,
		strings.Repeat("█", filled),
		colorGray,
		strings.Repeat("░", empty),
		colorReset,
	)
	return bar
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func printSeparator() {
	fmt.Printf("%s%s%s\n", colorGray, strings.Repeat("─", 52), colorReset)
}

var sysinfoCmd = &cobra.Command{
	Use:   "sysinfo",
	Short: "Display system information",
	Long:  `Display CPU, memory, disk, and Docker information for the current system.`,
	Run: func(cmd *cobra.Command, args []string) {
		initColors()
		ctx := context.Background()

		// ── Host ────────────────────────────────────────────────
		fmt.Println()
		fmt.Println(header("System"))
		printSeparator()
		if info, err := host.InfoWithContext(ctx); err == nil {
			uptime := time.Duration(info.Uptime) * time.Second
			h := int(uptime.Hours())
			m := int(uptime.Minutes()) % 60
			fmt.Printf("  %s %s%s%s\n", label("Hostname:"), colorBold, info.Hostname, colorReset)
			fmt.Printf("  %s %s\n", label("OS:"), fmt.Sprintf("%s %s (%s)", info.Platform, info.PlatformVersion, info.OS))
			fmt.Printf("  %s %s\n", label("Kernel:"), info.KernelVersion)
			fmt.Printf("  %s %dh %dm\n", label("Uptime:"), h, m)
			fmt.Printf("  %s %d\n", label("Procs:"), info.Procs)
		} else {
			fmt.Printf("  %s%s%s\n", colorRed, err.Error(), colorReset)
		}

		// ── CPU ─────────────────────────────────────────────────
		fmt.Println()
		fmt.Println(header("CPU"))
		printSeparator()
		if infos, err := cpu.InfoWithContext(ctx); err == nil && len(infos) > 0 {
			c := infos[0]
			cores, _ := cpu.CountsWithContext(ctx, true)
			logical, _ := cpu.CountsWithContext(ctx, false)
			fmt.Printf("  %s %s\n", label("Model:"), c.ModelName)
			fmt.Printf("  %s %d physical / %d logical\n", label("Cores:"), cores, logical)
			// gopsutil reports 0 or unreliable freq on Apple Silicon — skip if < 100 MHz
			if c.Mhz >= 100 {
				fmt.Printf("  %s %.0f MHz\n", label("Frequency:"), c.Mhz)
			}
		}
		if pcts, err := cpu.PercentWithContext(ctx, 500*time.Millisecond, false); err == nil && len(pcts) > 0 {
			pct := pcts[0]
			fmt.Printf("  %s %s %s%.1f%%%s\n",
				label("Usage:"),
				usageBar(pct, 20),
				colorBold, pct, colorReset,
			)
		}

		// ── Memory ──────────────────────────────────────────────
		fmt.Println()
		fmt.Println(header("Memory"))
		printSeparator()
		if v, err := mem.VirtualMemoryWithContext(ctx); err == nil {
			fmt.Printf("  %s %s / %s\n", label("RAM:"),
				formatBytes(v.Used), formatBytes(v.Total))
			fmt.Printf("  %s %s %s%.1f%%%s\n",
				label("Usage:"),
				usageBar(v.UsedPercent, 20),
				colorBold, v.UsedPercent, colorReset,
			)
			fmt.Printf("  %s %s\n", label("Available:"), formatBytes(v.Available))
		}
		if s, err := mem.SwapMemoryWithContext(ctx); err == nil && s.Total > 0 {
			fmt.Printf("  %s %s / %s\n", label("Swap:"),
				formatBytes(s.Used), formatBytes(s.Total))
		}

		// ── Disk ────────────────────────────────────────────────
		fmt.Println()
		fmt.Println(header("Disk"))
		printSeparator()
		if parts, err := disk.PartitionsWithContext(ctx, false); err == nil {
			printed := 0
			for _, p := range parts {
				mt := p.Mountpoint
				// On macOS keep only root and /Volumes/* ; skip APFS synthetic mounts
				if runtime.GOOS == "darwin" {
					if mt != "/" && !strings.HasPrefix(mt, "/Volumes/") {
						continue
					}
				}
				usage, err := disk.UsageWithContext(ctx, mt)
				if err != nil {
					continue
				}
				// skip tiny/virtual mounts (<1 GiB)
				if usage.Total < 1<<30 {
					continue
				}
				mount := mt
				if len(mount) > 20 {
					mount = "…" + mount[len(mount)-19:]
				}
				fmt.Printf("  %s %s / %s\n",
					label(mount+":"),
					formatBytes(usage.Used), formatBytes(usage.Total))
				fmt.Printf("  %s %s %s%.1f%%%s\n",
					label(""),
					usageBar(usage.UsedPercent, 20),
					colorBold, usage.UsedPercent, colorReset,
				)
				printed++
			}
			if printed == 0 {
				fmt.Printf("  %sno physical disks found%s\n", colorGray, colorReset)
			}
		}

		// ── Docker ──────────────────────────────────────────────
		fmt.Println()
		fmt.Println(header("Docker"))
		printSeparator()
		containers, err := docker.GetDockerStat()
		if err != nil || len(containers) == 0 {
			if err != nil && strings.Contains(err.Error(), "not available") {
				fmt.Printf("  %sDocker not available%s\n", colorGray, colorReset)
			} else if len(containers) == 0 {
				fmt.Printf("  %sNo running containers%s\n", colorGray, colorReset)
			} else {
				fmt.Printf("  %s%s%s\n", colorRed, err.Error(), colorReset)
			}
		} else {
			fmt.Printf("  %s%s%d running%s\n", label("Containers:"), colorGreen, len(containers), colorReset)
			for _, c := range containers {
				name := c.Name
				if name == "" {
					name = c.ContainerID[:12]
				}
				fmt.Printf("  %s  %s%s%s\n", colorGray, colorGreen, name, colorReset)
			}
		}

		fmt.Println()
	},
}

func init() {
	rootCmd.AddCommand(sysinfoCmd)
}
