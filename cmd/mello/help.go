package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"mework/internal/cli"
)

// registerCommands wires all top-level commands into the root. Command groups
// from later phases attach here; phase 01 ships only `config` + version.
func registerCommands() {
	configCmd.GroupID = groupAdditional
	rootCmd.AddCommand(configCmd)
}

var configCmd = &cobra.Command{
	Use:     "config",
	Short:   "Show or modify CLI configuration",
	GroupID: groupAdditional,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the resolved configuration (token masked)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := cli.LoadConfig(profile())
		if err != nil {
			return err
		}
		// Mask the token before display.
		masked := *cfg
		masked.Token = maskToken(cfg.Token)
		out, err := json.MarshalIndent(masked, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
}

// maskToken hides all but the last 4 chars of a secret for display.
func maskToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 4 {
		return "****"
	}
	return "****" + token[len(token)-4:]
}
