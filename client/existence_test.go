package client

import (
	"os/exec"
	"testing"
)

func TestClientPackagesExist(t *testing.T) {
	tests := []struct {
		name    string
		pkgPath string
	}{
		{name: "cli", pkgPath: "mework/client/cli"},
		{name: "runner", pkgPath: "mework/client/runner"},
		{name: "subscribe", pkgPath: "mework/client/subscribe"},
		{name: "workspacefs", pkgPath: "mework/client/workspacefs"},
		{name: "osproc", pkgPath: "mework/client/osproc"},
		{name: "cmd/mework", pkgPath: "mework/client/cmd/mework"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command("go", "list", "-find", tt.pkgPath)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("package %q should resolve but go list failed: %v\noutput: %s", tt.pkgPath, err, string(out))
				return
			}
			if len(out) == 0 {
				t.Errorf("package %q should resolve but go list returned empty", tt.pkgPath)
			}
		})
	}
}
