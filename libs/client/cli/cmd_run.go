package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"mework/libs/client/runner"
)

// runCmd sends a one-shot instruction to the running offline agent.
// Usage: mework run <instruction>
var runCmd = &cobra.Command{
	Use:   "run <instruction>",
	Short: "Send a one-shot task to the running offline agent",
	Long: `Sends a one-shot instruction to the currently running offline-mode agent.
The instruction is delivered over a Unix socket (never argv) to preserve
the injection-safety invariant. If no offline agent is running, the
command prints an error.

Examples:
  mework run "list files in the workspace"
  mework run "what is the current project status?"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		instruction := strings.Join(args, " ")
		out := cmd.OutOrStdout()

		// Find registered offline agent(s).
		agents, err := runner.ListOfflineAgents()
		if err != nil {
			return fmt.Errorf("list agents: %w", err)
		}
		if len(agents) == 0 {
			return fmt.Errorf("no offline agent running — run 'mework start --offline' first")
		}

		// Use the first available agent.
		agent := agents[0]
		if !runner.CheckAgentRunning(agent.SocketPath) {
			return fmt.Errorf("offline agent %q is registered but not reachable", agent.Name)
		}

		sender := resolveSender()
		output, exitCode, err := runner.SendInstructionResult(agent.SocketPath, instruction, sender)
		if err != nil {
			return err
		}
		if output != "" {
			fmt.Fprint(out, output)
		}
		if exitCode != 0 {
			return fmt.Errorf("task failed with exit code %d", exitCode)
		}
		return nil
	},
}
