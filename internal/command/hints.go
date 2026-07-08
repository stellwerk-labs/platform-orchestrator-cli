package command

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

type Cause int

const (
	HintCauseEnvNotFound     Cause = iota
	HintCauseEnvTypeNotFound Cause = iota
)

type Hint struct {
	commands    []HintCommand
	returnError bool
}

type HintCommand struct {
	cobraCommand *cobra.Command
	noExecution  bool
}

var (
	causesToHints map[Cause]Hint
)

// Use init to avoid initialization cycle.
func init() {
	causesToHints = map[Cause]Hint{
		HintCauseEnvNotFound: {
			commands: []HintCommand{
				{cobraCommand: ListEnvironments},
				{cobraCommand: CreateEnvironment, noExecution: true},
			},
			returnError: true,
		},
		HintCauseEnvTypeNotFound: {
			commands: []HintCommand{
				{cobraCommand: ListEnvironmentTypes},
				{cobraCommand: CreateEnvironmentType, noExecution: true},
			},
		},
	}
}

// SuggestHintByCause pretty print hint to solve the issue.
func SuggestHintByCause(ctx context.Context, cause Cause, parentCmd *cobra.Command, parentErr error) error {
	hint, found := causesToHints[cause]
	if !found {
		return parentErr
	}

	failureMessageF("Error: %s\n\n", parentErr.Error())
	infoMessageF("Suggestion:")

	for _, hintCommand := range hint.commands {
		_, _ = fmt.Fprintf(
			color.Output,
			"\t - %s using: %s\n",
			hintCommand.cobraCommand.Short,
			color.HiGreenString(hintCommand.cobraCommand.CommandPath()),
		)
	}

	infoMessageF("")

	noPromptFlag, _ := parentCmd.Flags().GetBool(deployCmdNoPromptFlag)
	if noPromptFlag || !isatty.IsTerminal(os.Stdin.Fd()) {
		return parentErr
	}

	for _, hintCommand := range hint.commands {
		if hintCommand.noExecution {
			continue
		}

		cmd := hintCommand.cobraCommand

		_, _ = fmt.Fprintf(
			color.Output,
			"Run %s right now? ",
			color.HiGreenString(cmd.CommandPath()),
		)

		if err := PromptTextAndEnterToContinue(ctx, os.Stdin, ""); err != nil {
			return err
		}

		if err := cmd.RunE(parentCmd, parentCmd.Flags().Args()); err != nil {
			return err
		}
	}

	if hint.returnError {
		return parentErr
	}

	return nil
}

// errorToHint trying to find best Hint to solve the error.
// Error message parsing is part of initial phase
func errorToHint(cmd *cobra.Command, parentErr error) error {
	if strings.Contains(parentErr.Error(), "environment type ") {
		return SuggestHintByCause(cmd.Context(), HintCauseEnvTypeNotFound, cmd, parentErr)
	}

	return parentErr
}
