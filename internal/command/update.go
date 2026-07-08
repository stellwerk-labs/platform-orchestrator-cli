package command

import (
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
)

var UpdateCmd = &cobra.Command{
	GroupID:       CrudGroup.ID,
	Use:           "update <type>",
	Short:         "Update an object of a given type",
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		out, _ := cmd.Flags().GetString(printer.OutputFormatFlag)
		ctx, err := withPrinter(cmd.Context(), out, []string{printer.JsonPrinterType, printer.YamlPrinterType})
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
	UpdateCmd.PersistentFlags().String(createUpdateCmdSetJsonFlag, "", "Set JSON input as either a raw string '{..}', stdin '-', or a @-prefixed file path")
	UpdateCmd.PersistentFlags().String(createUpdateCmdSetYamlFlag, "", "Set YAML input as either a raw string '{..}', stdin '-', or a @-prefixed yaml file path")
	UpdateCmd.PersistentFlags().StringArray(createUpdateCmdSetFlag, []string{}, "Set key=value pairs")
	printer.SetupSingleOutputFormatFlag(UpdateCmd.PersistentFlags())
	UpdateCmd.MarkFlagsMutuallyExclusive(createUpdateCmdSetYamlFlag, createUpdateCmdSetJsonFlag)
}
