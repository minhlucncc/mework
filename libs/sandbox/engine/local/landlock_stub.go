//go:build !linux

package local

import (
	"context"

	"mework/libs/shared/core"
	"mework/libs/shared/ports"
)

// landlockDriver is a stub on non-Linux platforms. newLandlockDriver returns
// nil, so these methods should never be called.
type landlockDriver struct{}

func (d *landlockDriver) Caps() core.SandboxCaps                 { return core.SandboxCaps{} }
func (d *landlockDriver) abiVersion() int                         { return 0 }
func (d *landlockDriver) Start(_ context.Context, _ core.RunSpec) (ports.Sandbox, error) {
	return nil, nil
}
func (d *landlockDriver) Stop(_ context.Context, _ string) error   { return nil }
func (d *landlockDriver) Destroy(_ context.Context, _ string) error { return nil }

// newLandlockDriver returns nil on non-Linux platforms.
func newLandlockDriver() *landlockDriver { return nil }
