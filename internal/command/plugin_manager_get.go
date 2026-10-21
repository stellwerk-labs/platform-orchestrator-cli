package command

import (
	"net/http"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
)

var ListPluginPackages = &cobra.Command{
	Use:   "plugin-packages",
	Args:  cobra.NoArgs,
	Short: "List the plugin packages registered in the Plugin Manager (platform operators only)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		r, err := MustCpClient(cmd.Context()).ListPluginPackagesWithResponse(cmd.Context())
		if err != nil {
			return errors.Wrap(err, "failed to list plugin packages")
		} else if r.StatusCode() != http.StatusOK || r.JSON200 == nil {
			return pluginManagerError("list plugin packages", r.StatusCode(), r.Body, "")
		}
		infoMessageF("Plugin Manager revision %d.", r.JSON200.Revision)
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), r.JSON200.Items)
	},
}

var ListPluginInstallations = &cobra.Command{
	Use:   "plugin-installations",
	Args:  cobra.NoArgs,
	Short: "List the plugin installations and their organizations (platform operators only)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		r, err := MustCpClient(cmd.Context()).ListPluginInstallationsWithResponse(cmd.Context())
		if err != nil {
			return errors.Wrap(err, "failed to list plugin installations")
		} else if r.StatusCode() != http.StatusOK || r.JSON200 == nil {
			return pluginManagerError("list plugin installations", r.StatusCode(), r.Body, "")
		}
		infoMessageF("Plugin Manager revision %d.", r.JSON200.Revision)
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), r.JSON200.Items)
	},
}

var GetPluginInstallation = &cobra.Command{
	Use:   "plugin-installation <installation-id>",
	Args:  cobra.ExactArgs(1),
	Short: "Get a plugin installation, its organizations and the last status read from the running plugin (platform operators only)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		id, err := parseInstallationID(args[0])
		if err != nil {
			return err
		}
		r, err := MustCpClient(cmd.Context()).GetPluginInstallationWithResponse(cmd.Context(), id)
		if err != nil {
			return errors.Wrap(err, "failed to get the plugin installation")
		} else if r.StatusCode() != http.StatusOK || r.JSON200 == nil {
			return pluginManagerError("get plugin installation "+id.String(), r.StatusCode(), r.Body, id.String())
		}
		printPluginInstallationState(*r.JSON200)
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *r.JSON200)
	},
}

var ListPluginAuditEvents = &cobra.Command{
	Use:   "plugin-audit-events",
	Args:  cobra.NoArgs,
	Short: "Page the append-only Plugin Manager audit trail, oldest first (platform operators only)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		after, _ := cmd.Flags().GetInt64(pluginAuditEventsAfterFlag)
		limit, _ := cmd.Flags().GetInt(pluginAuditEventsLimitFlag)
		r, err := MustCpClient(cmd.Context()).ListPluginAuditEventsWithResponse(cmd.Context(), &cp.ListPluginAuditEventsParams{After: &after, Limit: &limit})
		if err != nil {
			return errors.Wrap(err, "failed to list plugin audit events")
		} else if r.StatusCode() != http.StatusOK || r.JSON200 == nil {
			return pluginManagerError("list plugin audit events", r.StatusCode(), r.Body, "")
		}
		items := r.JSON200.Items
		if len(items) == limit {
			infoMessageF("More events may follow: --%s %d.", pluginAuditEventsAfterFlag, items[len(items)-1].Id)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), items)
	},
}

func init() {
	ListPluginAuditEvents.Flags().Int64(pluginAuditEventsAfterFlag, 0, "Only events after this event ID")
	ListPluginAuditEvents.Flags().Int(pluginAuditEventsLimitFlag, 100, "At most this many events (1 to 200)")
	GetCmd.AddCommand(ListPluginPackages, ListPluginInstallations, GetPluginInstallation, ListPluginAuditEvents)
}
