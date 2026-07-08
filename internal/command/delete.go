package command

import (
	"github.com/spf13/cobra"
)

var DeleteCmd = &cobra.Command{
	GroupID:       CrudGroup.ID,
	Use:           "delete <type>",
	Short:         "Delete an object of a given type",
	SilenceErrors: true,
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	},
}
