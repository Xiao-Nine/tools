/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var (
	lsAll  bool // 显示隐藏文件
	lsLong bool // 长格式显示
)

var lsCmd = &cobra.Command{
	Use:   "ls [path]",
	Short: "List directory contents",
	Long:  `List files and directories in the specified path (default: current directory).`,
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ls: %v\n", err)
			os.Exit(1)
		}

		// 过滤隐藏文件
		if !lsAll {
			filtered := make([]os.DirEntry, 0, len(entries))
			for _, e := range entries {
				if !strings.HasPrefix(e.Name(), ".") {
					filtered = append(filtered, e)
				}
			}
			entries = filtered
		}

		sort.Slice(entries, func(i, j int) bool {
			return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
		})

		if lsLong {
			for _, e := range entries {
				info, err := e.Info()
				if err != nil {
					fmt.Fprintf(os.Stderr, "ls: %v\n", err)
					continue
				}
				fmt.Printf("%s  %8d  %s  %s\n",
					info.Mode(),
					info.Size(),
					info.ModTime().Format("Jan 02 15:04"),
					e.Name(),
				)
			}
		} else {
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				name := e.Name()
				if e.IsDir() {
					name += "/"
				}
				names = append(names, name)
			}
			fmt.Println(strings.Join(names, "  "))
		}
	},
}

func init() {
	lsCmd.Flags().BoolVarP(&lsAll, "all", "a", false, "show hidden files")
	lsCmd.Flags().BoolVarP(&lsLong, "long", "l", false, "use long listing format")
	rootCmd.AddCommand(lsCmd)
}
