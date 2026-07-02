package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"github.com/spf13/cobra"

	"mework/libs/client/runner"
)

var chatAgentName string

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start an interactive chat session with a local agent",
	Long: `Start an interactive chat session with a local offline agent.

Messages are sent to the agent and responses are displayed in real-time.
Type /exit or Ctrl+C to quit.

Examples:
  mework chat --agent mybot
  mework chat -a mybot
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name := chatAgentName
		if name == "" && len(args) > 0 {
			name = args[0]
		}
		if name == "" {
			return fmt.Errorf("--agent is required (e.g. --agent mybot)")
		}

		// Look up the local offline agent.
		local, err := runner.LookupOfflineAgent(name)
		if err != nil || local == nil {
			return fmt.Errorf("offline agent %q not found — is the daemon running?", name)
		}
		if !runner.CheckAgentRunning(local.SocketPath) {
			return fmt.Errorf("offline agent %q is registered but not reachable", name)
		}

		sender := resolveSender()

		// Indicator for joined sandbox mode (updated atomically from response parsing).
		var joined atomic.Value
		joined.Store("")

		fmt.Printf("Chatting with %q. Type your message and press Enter. /exit to quit.\n", name)
		fmt.Println(strings.Repeat("-", 40))

		scanner := bufio.NewScanner(os.Stdin)
		for {
			// Show prompt with joined indicator if in sandbox mode.
			prefix := "> "
			if j := joined.Load().(string); j != "" {
				prefix = fmt.Sprintf("\033[33m[joined: %s]\033[0m > ", j)
			}
			fmt.Print(prefix)
			if !scanner.Scan() {
				break
			}
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if line == "/exit" || line == "/quit" {
				break
			}

			output, _, err := runner.SendInstructionResult(local.SocketPath, line, sender)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				continue
			}
			if output != "" {
				// Check for join/leave indicators in the response.
				if strings.HasPrefix(output, "Joined sandbox ") {
					sandboxID := strings.TrimPrefix(output, "Joined sandbox ")
					if idx := strings.IndexAny(sandboxID, ". "); idx > 0 {
						sandboxID = sandboxID[:idx]
					}
					joined.Store(sandboxID)
				} else if strings.HasPrefix(output, "Left sandbox mode") {
					joined.Store("")
				}

				// Print last non-empty line for cleaner display
				lines := strings.Split(strings.TrimSpace(output), "\n")
				for _, l := range lines {
					l = strings.TrimSpace(l)
					if l != "" {
						fmt.Println(l)
					}
				}
			}
			fmt.Println(strings.Repeat("-", 40))
		}
		return nil
	},
}

func init() {
	chatCmd.Flags().StringVarP(&chatAgentName, "agent", "a", "", "Agent name to chat with (required)")
	rootCmd.AddCommand(chatCmd)
}
