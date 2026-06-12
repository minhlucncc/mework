package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Print version information",
	GroupID: groupAdditional,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("mello %s\ncommit: %s\nbuilt:  %s\n", version, commit, date)
	},
}
