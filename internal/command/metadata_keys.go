package command

import (
	"fmt"
	"net/http"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/clients"
	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const metadataKeyUse = "metadata-key <name>"

var CreateMetadataKey = &cobra.Command{
	Use:   metadataKeyUse,
	Args:  cobra.ExactArgs(1),
	Short: "Create an organization metadata key",
	Long: fmt.Sprintf(`Create an organization metadata key.

The following fields can be set using --set, --set-json, or --set-yaml: %s.
The name positional argument takes precedence over a name in the request body.
`, generateTopLevelSetFields(dp.MetadataKeyCreateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		body, err := readSetFlagsIntoType[dp.MetadataKeyCreateBody](cmd)
		if err != nil {
			return err
		}
		body.Name = args[0]
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		res, err := MustDpClient(cmd.Context()).CreateMetadataKeyWithResponse(cmd.Context(), orgID, *body)
		if err != nil {
			return errors.Wrap(err, "failed to create metadata key")
		}
		switch res.StatusCode() {
		case http.StatusCreated:
			successMessageF("Metadata key %q created in organization %s.", args[0], orgID)
			return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *res.JSON201)
		case http.StatusBadRequest:
			return errors.Errorf("request is invalid: %s", res.JSON400.Message)
		case http.StatusNotFound:
			return errors.Errorf("organization '%s' not found", orgID)
		case http.StatusConflict:
			return errors.Errorf("metadata key %q already exists in organization '%s'", args[0], orgID)
		default:
			return errors.Errorf("unexpected status code %d when creating metadata key: %s", res.StatusCode(), string(res.Body))
		}
	},
}

var GetMetadataKey = &cobra.Command{
	Use:   metadataKeyUse,
	Args:  cobra.ExactArgs(1),
	Short: "Get an organization metadata key",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		res, err := MustDpClient(cmd.Context()).GetMetadataKeyWithResponse(cmd.Context(), orgID, args[0])
		if err != nil {
			return errors.Wrap(err, "failed to get metadata key")
		}
		switch res.StatusCode() {
		case http.StatusOK:
			return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *res.JSON200)
		case http.StatusNotFound:
			return errors.Errorf("metadata key %q not found in organization '%s'", args[0], orgID)
		default:
			return errors.Errorf("unexpected status code %d when getting metadata key: %s", res.StatusCode(), string(res.Body))
		}
	},
}

var ListMetadataKeys = &cobra.Command{
	Use:   "metadata-keys",
	Args:  cobra.NoArgs,
	Short: "List organization metadata keys",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		items, err := clients.CollectAll(
			func(page string) (*dp.ListMetadataKeysResponse, error) {
				return MustDpClient(cmd.Context()).ListMetadataKeysWithResponse(cmd.Context(), orgID, &dp.ListMetadataKeysParams{Page: ref.RefStringEmptyNil(page)})
			},
			func(res *dp.ListMetadataKeysResponse) ([]dp.MetadataKey, *string) {
				return res.JSON200.Items, res.JSON200.NextPageToken
			},
		)
		if err != nil {
			return err
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), items)
	},
}

var UpdateMetadataKey = &cobra.Command{
	Use:   metadataKeyUse,
	Args:  cobra.ExactArgs(1),
	Short: "Update an organization metadata key",
	Long: fmt.Sprintf(`Update an organization metadata key.

The following fields can be set using --set, --set-json, or --set-yaml: %s.
`, generateTopLevelSetFields(dp.MetadataKeyUpdateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		body, err := readSetFlagsIntoType[dp.MetadataKeyUpdateBody](cmd)
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		res, err := MustDpClient(cmd.Context()).UpdateMetadataKeyWithResponse(cmd.Context(), orgID, args[0], *body)
		if err != nil {
			return errors.Wrap(err, "failed to update metadata key")
		}
		switch res.StatusCode() {
		case http.StatusOK:
			changedMessageF("Metadata key %q updated in organization %s.", args[0], orgID)
			return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *res.JSON200)
		case http.StatusBadRequest:
			return errors.Errorf("request is invalid: %s", res.JSON400.Message)
		case http.StatusNotFound:
			return errors.Errorf("metadata key %q not found in organization '%s'", args[0], orgID)
		default:
			return errors.Errorf("unexpected status code %d when updating metadata key: %s", res.StatusCode(), string(res.Body))
		}
	},
}

var DeleteMetadataKey = &cobra.Command{
	Use:   metadataKeyUse,
	Args:  cobra.ExactArgs(1),
	Short: "Delete an organization metadata key",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		res, err := MustDpClient(cmd.Context()).DeleteMetadataKeyWithResponse(cmd.Context(), orgID, args[0])
		if err != nil {
			return errors.Wrap(err, "failed to delete metadata key")
		}
		switch res.StatusCode() {
		case http.StatusNoContent:
			changedMessageF("Metadata key %q deleted from organization %s.", args[0], orgID)
			return nil
		case http.StatusNotFound:
			return errors.Errorf("metadata key %q not found in organization '%s'", args[0], orgID)
		default:
			return errors.Errorf("unexpected status code %d when deleting metadata key: %s", res.StatusCode(), string(res.Body))
		}
	},
}

func init() {
	CreateCmd.AddCommand(CreateMetadataKey)
	GetCmd.AddCommand(GetMetadataKey, ListMetadataKeys)
	UpdateCmd.AddCommand(UpdateMetadataKey)
	DeleteCmd.AddCommand(DeleteMetadataKey)
}
