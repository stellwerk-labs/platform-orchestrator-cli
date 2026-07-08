package main

import (
	"context"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/command"
)

func main() {
	ctx := context.Background()
	cobra.EnableTraverseRunHooks = true

	if err := command.RootCmd.ExecuteContext(ctx); err != nil {
		color.HiRed("Error: %s", err.Error())
		os.Exit(1)
	}
}
