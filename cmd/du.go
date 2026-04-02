package cmd

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var (
	duAll   bool
	duDepth int
)

type fileEntry struct {
	name  string
	size  int64
	isDir bool
}

func calcSize(path string) (int64, error) {
	var total int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

var duCmd = &cobra.Command{
	Use:   "du [path]",
	Short: "Display file and directory sizes with a bar chart",
	Long:  `Display the size of each file and directory under the given path, sorted by size with a horizontal bar chart for visual comparison.`,
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		initColors()

		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}

		absDir, err := filepath.Abs(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading directory: %v\n", err)
			os.Exit(1)
		}

		var files []fileEntry
		var totalSize int64

		for _, e := range entries {
			name := e.Name()
			if !duAll && strings.HasPrefix(name, ".") {
				continue
			}

			path := filepath.Join(dir, name)
			var size int64

			if e.IsDir() {
				size, _ = calcSize(path)
			} else {
				info, err := e.Info()
				if err != nil {
					continue
				}
				size = info.Size()
			}

			files = append(files, fileEntry{name: name, size: size, isDir: e.IsDir()})
			totalSize += size
		}

		sort.Slice(files, func(i, j int) bool {
			return files[i].size > files[j].size
		})

		// Find max name length for alignment
		maxName := 0
		for _, f := range files {
			n := len(f.name)
			if f.isDir {
				n++ // for trailing /
			}
			if n > maxName {
				maxName = n
			}
		}
		if maxName < 8 {
			maxName = 8
		}

		fmt.Printf("\n%s%s📁 Directory: %s%s (Total: %s)\n",
			colorBold, colorCyan, absDir, colorReset, formatBytes(uint64(totalSize)))
		fmt.Printf("%s%s%s\n", colorGray, strings.Repeat("─", maxName+60), colorReset)

		const barWidth = 40

		for _, f := range files {
			displayName := f.name
			if f.isDir {
				displayName += "/"
			}

			var pct float64
			if totalSize > 0 {
				pct = float64(f.size) / float64(totalSize) * 100
			}

			filled := int(math.Round(pct / 100.0 * float64(barWidth)))
			if filled > barWidth {
				filled = barWidth
			}
			empty := barWidth - filled

			var barColor string
			switch {
			case pct >= 50:
				barColor = colorRed
			case pct >= 20:
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

			sizeStr := formatBytes(uint64(f.size))

			fmt.Printf("  %-*s %10s %s %5.1f%%\n",
				maxName, displayName, sizeStr, bar, pct)
		}

		if len(files) == 0 {
			fmt.Printf("  %s(empty)%s\n", colorGray, colorReset)
		}
		fmt.Println()
	},
}

func init() {
	duCmd.Flags().BoolVarP(&duAll, "all", "a", false, "include hidden files and directories")
	duCmd.Flags().IntVarP(&duDepth, "depth", "d", 1, "depth level (reserved for future use)")
	rootCmd.AddCommand(duCmd)
}
