package command

import (
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
)

var RegenerateCmd = &cobra.Command{
	GroupID:       CrudGroup.ID,
	Use:           "regenerate <type>",
	Short:         "Regenerate a credential for an object",
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		out, _ := cmd.Flags().GetString(printer.OutputFormatFlag)
		ctx, err := withPrinter(cmd.Context(), out, []string{printer.JsonPrinterType, printer.YamlPrinterType})
		if err != nil {
			return err
		}
		cmd.SetContext(ctx)
		return nil
	},
	CompletionOptions: cobra.CompletionOptions{HiddenDefaultCmd: true},
}

func init() {
	printer.SetupSingleOutputFormatFlag(RegenerateCmd.PersistentFlags())
	RootCmd.AddCommand(RegenerateCmd)
}
