package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version marks the current operator-facing release line. The scout series
// corresponds to the shipped Tool Registry + Telegram Scout + thin resume set.
const Version = "0.2.0-scout"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print gormes version",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println("gormes", Version)
	},
}
