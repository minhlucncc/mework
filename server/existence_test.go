package server

import (
	"os/exec"
	"testing"
)

func TestServerPackagesExist(t *testing.T) {
	tests := []struct {
		name    string
		pkgPath string
	}{
		{name: "hub", pkgPath: "mework/server/hub"},
		{name: "registry", pkgPath: "mework/server/registry"},
		{name: "session", pkgPath: "mework/server/session"},
		{name: "catalog", pkgPath: "mework/server/catalog"},
		{name: "orchestrator", pkgPath: "mework/server/orchestrator"},
		{name: "webhook", pkgPath: "mework/server/webhook"},
		{name: "writeback", pkgPath: "mework/server/writeback"},
		{name: "auth", pkgPath: "mework/server/auth"},
		{name: "middleware", pkgPath: "mework/server/middleware"},
		{name: "platform/store", pkgPath: "mework/server/platform/store"},
		{name: "platform/secret", pkgPath: "mework/server/platform/secret"},
		{name: "platform/token", pkgPath: "mework/server/platform/token"},
		{name: "bus", pkgPath: "mework/server/bus"},
		{name: "storage", pkgPath: "mework/server/storage"},
		{name: "provider", pkgPath: "mework/server/provider"},
		{name: "cmd/mework-server", pkgPath: "mework/server/cmd/mework-server"},
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
