package command

import (
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
)

var GetCmd = &cobra.Command{
	GroupID:       CrudGroup.ID,
	Use:           "get <type>",
	Short:         "Get or list objects of a given type",
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		out, _ := cmd.Flags().GetString(printer.OutputFormatFlag)
		ctx, err := withPrinter(cmd.Context(), out, []string{printer.JsonPrinterType, printer.YamlPrinterType, printer.TablePrinterType})
		if err != nil {
			return err
		}
		cmd.SetContext(ctx)
		return nil
	},
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	},
}

func init() {
	printer.SetupListOutputFormatFlag(GetCmd.PersistentFlags())
}
