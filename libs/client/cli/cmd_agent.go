package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// agentCmd is the parent grouping command for agent operations.
// In production, `mework agent list` is processed here; in tests
// agentListCmd is executed directly (no parent) so cobra's Execute
// does not delegate to the root command.
var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "List and interact with agents (unit queues)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return agentListCmd.RunE(cmd, args)
	},
}

// agentRow is a decode target matching unitqueue.Registration JSON.
type agentRow struct {
	Name      string `json:"name"`
	SessionID string `json:"session_id"`
	RunnerID  string `json:"runner_id"`
	Status    string `json:"status"`
	Created   string `json:"created"`
}

// agentListCmd lists agents from the catalog and unit queues.
var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available agents and online unit queues",
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		base, token, err := sessionEndpoint()
		if err != nil {
			return err
		}

		showJSON, _ := cmd.Flags().GetBool("json")

		// Fetch unit queues (online agents).
		var queues []agentRow
		status, err := sessionDo(http.MethodGet, base+"/api/v1/unitqueues", token, nil, &queues)
		if err != nil || status != http.StatusOK {
			queues = []agentRow{}
		}

		if showJSON {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			return enc.Encode(queues)
		}

		tw := newTableTo(out)
		row(tw, "NAME", "SESSION ID", "RUNNER", "STATUS")
		if len(queues) == 0 {
			row(tw, "(no online agents)")
		}
		for _, q := range queues {
			row(tw, q.Name, q.SessionID, q.RunnerID, q.Status)
		}
		return tw.Flush()
	},
}

// agentSendCmd sends a chat message to a named agent (unit queue).
// Usage: mework agent send <name> <message>
var agentSendCmd = &cobra.Command{
	Use:   "send <name> <message>",
	Short: "Send a chat message to an agent by name (unit queue)",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		message := strings.Join(args[1:], " ")
		out := cmd.OutOrStdout()

		base, token, err := sessionEndpoint()
		if err != nil {
			return err
		}

		waitMode, _ := cmd.Flags().GetBool("wait")

		// If --wait, first look up the agent to get the session ID, so we can
		// subscribe to the SSE stream before sending to avoid race conditions.
		if waitMode {
			var reg agentRow
			_, err := sessionDo(http.MethodGet, base+"/api/v1/unitqueues/"+name, token, nil, &reg)
			if err != nil {
				return fmt.Errorf("agent %q not found: %w", name, err)
			}

			// Subscribe to the session's SSE stream in a goroutine.
			frames := make(chan string, 64)
			errCh := make(chan error, 1)
			go func() {
				req, rerr := http.NewRequest(http.MethodGet, base+"/api/v1/sessions/"+reg.SessionID+"/stream", nil)
				if rerr != nil {
					errCh <- rerr
					return
				}
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("Accept", "text/event-stream")

				resp, rerr := (&http.Client{}).Do(req)
				if rerr != nil {
					errCh <- rerr
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					errCh <- fmt.Errorf("attach: status %d", resp.StatusCode)
					return
				}

				scanner := bufio.NewScanner(resp.Body)
				scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
				var data string
				for scanner.Scan() {
					line := scanner.Text()
					switch {
					case strings.HasPrefix(line, "data: "):
						data = strings.TrimPrefix(line, "data: ")
					case line == "":
						if data != "" {
							frames <- data
							data = ""
						}
					}
				}
				close(frames)
			}()

			// Send the message.
			body := map[string]string{"role": "user", "content": message}
			status, serr := sessionDo(http.MethodPost, base+"/api/v1/unitqueues/"+name+"/messages", token, body, nil)
			if serr != nil {
				return fmt.Errorf("send: %w", serr)
			}
			if status != http.StatusAccepted {
				return fmt.Errorf("send: unexpected status %d", status)
			}

			// Wait for response events on the SSE stream.
			idle := time.NewTimer(120 * time.Second)
			defer idle.Stop()

			for {
				select {
				case evJSON, ok := <-frames:
					if !ok {
						return nil
					}
					var ev struct {
						Kind    string `json:"kind"`
						Content string `json:"content,omitempty"`
					}
					if json.Unmarshal([]byte(evJSON), &ev) != nil {
						continue
					}
					switch ev.Kind {
					case "token", "message":
						if ev.Content != "" {
							fmt.Fprint(out, ev.Content)
						}
					case "done":
						fmt.Fprintln(out)
						return nil
					case "error":
						fmt.Fprintln(out)
						if ev.Content != "" {
							return fmt.Errorf("agent error: %s", ev.Content)
						}
						return fmt.Errorf("agent error")
					}
				case err := <-errCh:
					return err
				case <-idle.C:
					fmt.Fprintln(out)
					return fmt.Errorf("timeout waiting for agent response")
				}
				if !idle.Stop() {
					select { case <-idle.C: default: }
				}
				idle.Reset(120 * time.Second)
			}
		}

		// Non-wait mode: just send and print accepted.
		body := map[string]string{"role": "user", "content": message}
		status, err := sessionDo(http.MethodPost, base+"/api/v1/unitqueues/"+name+"/messages", token, body, nil)
		if err != nil {
			return err
		}
		if status != http.StatusAccepted {
			return fmt.Errorf("send: unexpected status %d", status)
		}
		showJSON, _ := cmd.Flags().GetBool("json")
		if showJSON {
			return json.NewEncoder(out).Encode(map[string]string{"status": "accepted", "name": name})
		}
		fmt.Fprintf(out, "message sent to %q\n", name)
		return nil
	},
}

func init() {
	agentListCmd.Flags().Bool("json", false, "Output as JSON")
	agentSendCmd.Flags().BoolP("wait", "w", false, "Wait for the agent's response via SSE")
	agentSendCmd.Flags().Bool("json", false, "Output as JSON")
	agentCmd.AddCommand(agentListCmd, agentSendCmd)
}
