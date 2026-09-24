package main

import (
	"os"

	"github.com/agentzhao/tricount-cli/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
