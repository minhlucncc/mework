package cli

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
)

func TestFlagOrEnvPrecedence(t *testing.T) {
	const env = "MELLO_TEST_VAL"

	newCmd := func() *cobra.Command {
		c := &cobra.Command{Use: "x"}
		c.Flags().String("val", "", "")
		return c
	}

	t.Run("flag wins over env and fallback", func(t *testing.T) {
		t.Setenv(env, "from-env")
		cmd := newCmd()
		_ = cmd.Flags().Set("val", "from-flag")
		if got := FlagOrEnv(cmd, "val", env, "fallback"); got != "from-flag" {
			t.Fatalf("want from-flag, got %q", got)
		}
	})

	t.Run("env wins when flag unchanged", func(t *testing.T) {
		t.Setenv(env, "from-env")
		cmd := newCmd()
		if got := FlagOrEnv(cmd, "val", env, "fallback"); got != "from-env" {
			t.Fatalf("want from-env, got %q", got)
		}
	})

	t.Run("fallback when neither set", func(t *testing.T) {
		os.Unsetenv(env)
		cmd := newCmd()
		if got := FlagOrEnv(cmd, "val", env, "fallback"); got != "fallback" {
			t.Fatalf("want fallback, got %q", got)
		}
	})
}

func TestResolveBaseURLDefault(t *testing.T) {
	os.Unsetenv("MELLO_BASE_URL")
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("server-url", "", "")
	if got := ResolveBaseURL(cmd, &Config{}); got != DefaultBaseURL {
		t.Fatalf("want default %q, got %q", DefaultBaseURL, got)
	}
	if got := ResolveBaseURL(cmd, &Config{BaseURL: "https://custom"}); got != "https://custom" {
		t.Fatalf("want config value, got %q", got)
	}
}
