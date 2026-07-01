package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"mework/libs/shared/config"
)

var providerMezonCmd = &cobra.Command{
	Use:   "mezon",
	Short: "Configure Mezon provider and bot credentials",
}

// mezonCredentials is the JSON shape persisted at
// <MEWORK_HOME>/provider/mezon/credentials.json. This is the file the
// offline-stack orchestrator (libs/client/runner/offline_stack.go) reads
// when it boots mework-mezon-worker, so the format is part of the
// documented contract.
type mezonCredentials struct {
	AppID   string `json:"app_id"`
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url,omitempty"`
}

// mezonCredentialPath is <MEWORK_HOME>/provider/mezon/credentials.json.
func mezonCredentialPath() string {
	return filepath.Join(config.MeworkDir(), "provider", "mezon", "credentials.json")
}

var providerMezonSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Store Mezon bot credentials locally",
	Long: `Store Mezon bot credentials for the standalone mework-mezon-worker.

Credentials are saved to <MEWORK_HOME>/provider/mezon/credentials.json with
mode 0600 so the offline-stack orchestrator can read them safely.

Required: --app-id, --api-key
Optional: --base-url (default: https://api.mezon.vn)
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		appID := FlagOrEnv(cmd, "app-id", "MEZON_APP_ID", "")
		apiKey := FlagOrEnv(cmd, "api-key", "MEZON_API_KEY", "")
		baseURL := FlagOrEnv(cmd, "base-url", "MEZON_BASE_URL", "")

		if appID == "" {
			return fmt.Errorf("--app-id is required (or set MEZON_APP_ID)")
		}
		if apiKey == "" {
			return fmt.Errorf("--api-key is required (or set MEZON_API_KEY)")
		}

		body, err := json.MarshalIndent(mezonCredentials{
			AppID:   appID,
			APIKey:  apiKey,
			BaseURL: baseURL,
		}, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal credentials: %w", err)
		}

		path := mezonCredentialPath()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("create credentials dir: %w", err)
		}
		// Explicit mode 0600 so the file is private to the owner — the
		// offline-stack orchestrator refuses to read credentials.json that
		// are group- or world-readable.
		if err := os.WriteFile(path, body, 0o600); err != nil {
			return fmt.Errorf("write credentials: %w", err)
		}

		fmt.Println("mezon credentials saved")
		return nil
	},
}

var providerMezonShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show stored Mezon credentials (masked)",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := mezonCredentialPath()
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("no mezon credentials configured")
				fmt.Println("run: mework provider mezon set --app-id <id> --api-key <key>")
				return nil
			}
			return fmt.Errorf("stat credentials: %w", err)
		}
		// Fail closed on insecure permissions so the user fixes the
		// file rather than silently leaking the API key.
		if info.Mode().Perm() != 0o600 {
			return fmt.Errorf("refusing to read credentials file with insecure permissions: %s has mode %#o (want 0600); chmod 600 it to continue", path, info.Mode().Perm())
		}

		body, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read credentials: %w", err)
		}
		var cred mezonCredentials
		if err := json.Unmarshal(body, &cred); err != nil {
			return fmt.Errorf("parse credentials: %w", err)
		}

		maskedKey := cred.APIKey
		if len(maskedKey) > 8 {
			maskedKey = maskedKey[:4] + "****" + maskedKey[len(maskedKey)-4:]
		}

		fmt.Printf("App ID:  %s\n", cred.AppID)
		fmt.Printf("API Key: %s\n", maskedKey)
		if cred.BaseURL != "" {
			fmt.Printf("Base URL: %s\n", cred.BaseURL)
		} else {
			fmt.Println("Base URL: (default https://api.mezon.vn)")
		}
		return nil
	},
}

func init() {
	providerMezonSetCmd.Flags().String("app-id", "", "Mezon app ID")
	providerMezonSetCmd.Flags().String("api-key", "", "Mezon API key")
	providerMezonSetCmd.Flags().String("base-url", "", "Mezon API base URL (default: https://api.mezon.vn)")
	providerMezonCmd.AddCommand(providerMezonSetCmd, providerMezonShowCmd)
	providerCmd.AddCommand(providerMezonCmd)
}
