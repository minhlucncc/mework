package sandbox

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"mework/libs/shared/core"
)

// TestSandboxBundleMetadata_Validate exercises the definition validation rules
// (task 1.2): name and version are required; the engine must be a known engine;
// the backend must be non-empty; container engines (docker/cloudflare/custom)
// require a pinned image while the local engine ignores it.
func TestSandboxBundleMetadata_Validate(t *testing.T) {
	tests := []struct {
		name    string
		meta    SandboxBundleMetadata
		wantErr bool
	}{
		{
			name: "valid local claude",
			meta: SandboxBundleMetadata{
				Name:    "local-claude",
				Version: "1.0.0",
				Engine:  "local",
				Backend: "claude",
			},
			wantErr: false,
		},
		{
			name: "missing name is rejected",
			meta: SandboxBundleMetadata{
				Version: "1.0.0",
				Engine:  "local",
				Backend: "claude",
			},
			wantErr: true,
		},
		{
			name: "missing version is rejected",
			meta: SandboxBundleMetadata{
				Name:    "local-claude",
				Engine:  "local",
				Backend: "claude",
			},
			wantErr: true,
		},
		{
			name: "unknown engine is rejected",
			meta: SandboxBundleMetadata{
				Name:    "bogus-claude",
				Version: "1.0.0",
				Engine:  "bogus",
				Backend: "claude",
			},
			wantErr: true,
		},
		{
			name: "empty backend is rejected",
			meta: SandboxBundleMetadata{
				Name:    "local-claude",
				Version: "1.0.0",
				Engine:  "local",
				Backend: "",
			},
			wantErr: true,
		},
		{
			name: "container engine without image is rejected",
			meta: SandboxBundleMetadata{
				Name:    "docker-claude",
				Version: "1.0.0",
				Engine:  "docker",
				Backend: "claude",
				Image:   "",
			},
			wantErr: true,
		},
		{
			name: "container engine with image is accepted",
			meta: SandboxBundleMetadata{
				Name:    "docker-claude",
				Version: "1.0.0",
				Engine:  "docker",
				Backend: "claude",
				Image:   "mework/claude:1.0.0",
			},
			wantErr: false,
		},
		{
			name: "local engine with empty image is accepted (image ignored)",
			meta: SandboxBundleMetadata{
				Name:    "local-claude",
				Version: "1.0.0",
				Engine:  "local",
				Backend: "claude",
				Image:   "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.meta.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("Validate() = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestSandboxBundleMetadata_AccessTierValidation(t *testing.T) {
	tests := []struct {
		name    string
		meta    SandboxBundleMetadata
		wantErr bool
		errHint string
	}{
		{
			name:    "empty AccessTier is valid (normalizes to worker)",
			meta:    SandboxBundleMetadata{Name: "test", Version: "1.0.0", Engine: "local", Backend: "claude", AccessTier: ""},
			wantErr: false,
		},
		{
			name:    "observer is valid",
			meta:    SandboxBundleMetadata{Name: "test", Version: "1.0.0", Engine: "local", Backend: "claude", AccessTier: core.AccessObserver},
			wantErr: false,
		},
		{
			name:    "worker is valid",
			meta:    SandboxBundleMetadata{Name: "test", Version: "1.0.0", Engine: "local", Backend: "claude", AccessTier: core.AccessWorker},
			wantErr: false,
		},
		{
			name:    "isolated is valid",
			meta:    SandboxBundleMetadata{Name: "test", Version: "1.0.0", Engine: "local", Backend: "claude", AccessTier: core.AccessIsolated},
			wantErr: false,
		},
		{
			name:    "unknown tier is rejected",
			meta:    SandboxBundleMetadata{Name: "test", Version: "1.0.0", Engine: "local", Backend: "claude", AccessTier: core.AccessTier("admin")},
			wantErr: true,
			errHint: "unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.meta.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("Validate() = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
			if tt.errHint != "" && err != nil && !strings.Contains(err.Error(), tt.errHint) {
				t.Errorf("Validate() error = %q, want hint %q", err.Error(), tt.errHint)
			}
		})
	}
}

// TestSandboxBundleMetadata_AccessTierRoundTrip asserts that the AccessTier
// field survives a marshal/unmarshal round-trip through both YAML and JSON.
func TestSandboxBundleMetadata_AccessTierRoundTrip(t *testing.T) {
	want := SandboxBundleMetadata{
		Name:       "test",
		Version:    "1.0.0",
		Engine:     "local",
		Backend:    "claude",
		AccessTier: core.AccessObserver,
	}

	// YAML round-trip
	yamlData, err := yaml.Marshal(want)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	var gotYAML SandboxBundleMetadata
	if err := yaml.Unmarshal(yamlData, &gotYAML); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if gotYAML.AccessTier != want.AccessTier {
		t.Errorf("YAML round-trip AccessTier = %q, want %q", gotYAML.AccessTier, want.AccessTier)
	}

	// JSON round-trip
	jsonData, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var gotJSON SandboxBundleMetadata
	if err := json.Unmarshal(jsonData, &gotJSON); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if gotJSON.AccessTier != want.AccessTier {
		t.Errorf("JSON round-trip AccessTier = %q, want %q", gotJSON.AccessTier, want.AccessTier)
	}
}

// TestSandboxBundleMetadata_YAMLRoundTrip asserts the new binding fields
// (Engine, Image, ResourceLimits) survive a marshal/unmarshal round-trip so a
// definition can be stored and resolved as catalog content without loss.
func TestSandboxBundleMetadata_YAMLRoundTrip(t *testing.T) {
	want := SandboxBundleMetadata{
		Name:    "docker-claude",
		Version: "2.1.0",
		Spec:    "v1",
		Engine:  "docker",
		Backend: "claude",
		Image:   "mework/claude:2.1.0",
		Author:  "ops",
		ResourceLimits: &core.ResourceLimits{
			CPU:    "2",
			Memory: "4Gi",
			Disk:   "10Gi",
		},
	}

	data, err := yaml.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got SandboxBundleMetadata
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.Engine != want.Engine {
		t.Errorf("Engine = %q, want %q", got.Engine, want.Engine)
	}
	if got.Image != want.Image {
		t.Errorf("Image = %q, want %q", got.Image, want.Image)
	}
	if got.ResourceLimits == nil {
		t.Fatalf("ResourceLimits = nil, want round-tripped value")
	}
	if got.ResourceLimits.CPU != want.ResourceLimits.CPU ||
		got.ResourceLimits.Memory != want.ResourceLimits.Memory ||
		got.ResourceLimits.Disk != want.ResourceLimits.Disk {
		t.Errorf("ResourceLimits = %+v, want %+v", got.ResourceLimits, want.ResourceLimits)
	}
}
