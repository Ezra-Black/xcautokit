package main

import (
	"fmt"
	"os"

	"github.com/xcautokit/xcautokit/internal/cli"
)

func main() {
	// Default to MCP when launched with no args (MCP clients often omit subcommands).
	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"mcp"}
	}
	if err := cli.Run(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
