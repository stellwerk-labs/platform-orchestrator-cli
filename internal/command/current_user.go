package command

import (
	"net/http"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var GetCurrentUser = &cobra.Command{
	Use:   "current-user",
	Args:  cobra.NoArgs,
	Short: "Get the details of the current authenticated user and their organizations",

	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		client := MustIamClient(cmd.Context())

		cu, err := client.GetCurrentUserWithResponse(cmd.Context())
		if err != nil {
			return err
		} else if cu.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when getting current user: %s", cu.StatusCode(), string(cu.Body))
		}
		printer := MustPrinter(cmd.Context())
		return printer.Write(cmd.OutOrStdout(), *cu.JSON200)
	},
}

func init() {
	GetCmd.AddCommand(GetCurrentUser)
}
