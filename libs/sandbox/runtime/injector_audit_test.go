package runtime

import (
	"context"
	"os"
	"strings"
	"testing"

	"mework/libs/shared/ports"
)

// TestAudit_SecretInjectorWritesSourceNotValue documents gap 0.9: the injector
// writes secret.Source (the identifier) to the file instead of secret.Value.
//
// AUDIT: This test MUST PASS today if the bug exists (file contains Source),
// and will FAIL after fix. It also verifies the fix works.
func TestAudit_SecretInjectorWritesValueNotSource(t *testing.T) {
	// Use a temp dir so tests don't need /run/mework/secrets
	inj := NewSecretInjector(t.TempDir())
	sandboxID := "audit-inject-" + t.Name()

	// Inject a secret with distinct Source and Value
	err := inj.Inject(context.Background(), sandboxID, []string{"test-source"}, []ports.SecretRef{
		{Name: "my-key", Source: "test-source", Value: "actual-secret-value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer inj.Cleanup(context.Background(), sandboxID)

	filePath := inj.SecretFilePath(sandboxID, "my-key")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}

	content := strings.TrimSpace(string(data))
	if content == "actual-secret-value" {
		// FIXED: the file contains the actual value
		t.Log("OK: secret file contains the actual Value")
	} else {
		t.Errorf("BUG: secret file contains %q, expected %q", content, "actual-secret-value")
	}
}
