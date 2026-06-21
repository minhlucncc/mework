// Package custom provides a way to plug in a custom sandbox engine from an
// external Go module. It implements the SandboxDriver contract and is wired
// by blank-importing the implementation.
//
// This is a stub — the full implementation lands in the sandbox-runtime change.
package custom

import "fmt"

func init() {
	fmt.Println("custom engine stub registered")
}
