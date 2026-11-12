package command

import (
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
)

const runnerLogsKind = "logs"

func setupRunnerLogsFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("raw", false, "Return original log bytes without formatting or terminal-control escaping")
	cmd.Flags().Bool("pretty", false, "Format decrypted logs even when output is redirected (pipes use original bytes by default)")
	cmd.MarkFlagsMutuallyExclusive("raw", "pretty")
}

func runnerLogsBody(cmd *cobra.Command, body []byte) []byte {
	raw, _ := cmd.Flags().GetBool("raw")
	pretty, _ := cmd.Flags().GetBool("pretty")
	if raw || cmd.Flags().Changed(printer.OutputFormatFlag) || (!pretty && !printer.RunnerLogsTerminal(cmd.OutOrStdout())) {
		return body
	}
	return printer.FormatRunnerLogs(body, printer.RunnerLogsColor(cmd.OutOrStdout()))
}
