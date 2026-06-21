// Package cloudflare implements the SandboxDriver interface for Cloudflare
// Workers AI / remote sandbox execution. The driver dispatches agent runs
// to Cloudflare's edge network for isolated, short-lived execution.
//
// This is a stub — the full implementation lands in the sandbox-runtime change.
// The cloudflare SDK, if any, lives only in this subpackage.
package cloudflare

import "fmt"

func init() {
	fmt.Println("cloudflare engine stub registered")
}
