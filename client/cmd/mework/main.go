package main

import (
	"mework/client/cli"

	// Blank-import sandbox engine drivers.
	_ "mework/sandbox/engine/local"
)

func main() {
	cli.Execute()
}
