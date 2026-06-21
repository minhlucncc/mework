// Package docker implements the SandboxDriver interface for Docker containers.
// The driver selects an image, creates and starts a container, mounts the
// workspace, executes the agent binary, captures output, and tears down.
// Capabilities: full process isolation, resource limits, configurable images.
//
// This is a stub — the full implementation lands in the sandbox-runtime change.
// No docker SDK dependency is imported here yet; the SDK lives only in this
// subpackage when implemented.
package docker

import "fmt"

func init() {
	fmt.Println("docker engine stub registered")
}
