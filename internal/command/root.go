package command

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/internal"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/config"
)

const defaultApiUrl = "https://api.stellwerk.localhost"

var versionCheckResult <-chan *internal.VersionCheckResult

var RootCmd = &cobra.Command{
	Use:           "octl",
	SilenceErrors: true,
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	},
	PersistentPreRunE: rootPersistentPreRunE,
	PersistentPostRun: rootPersistentPostRun,
}

func rootPersistentPreRunE(cmd *cobra.Command, args []string) error {
	d, _ := cmd.Flags().GetBool("debug")
	d = d || strings.ToLower(os.Getenv("PO_CLI_DEBUG")) == stringTrue
	internal.SetupLogging(d, cmd.ErrOrStderr())
	color.Output = cmd.ErrOrStderr()

	cfg, err := config.ReadFile()
	if err != nil {
		return errors.Wrap(err, "failed to read configuration")
	}
	if v, ok := os.LookupEnv("PO_ORG_ID"); ok {
		cfg.DefaultOrg = v
	}

	if v, ok := os.LookupEnv("PO_API_URL"); ok {
		if apiUrl, err := url.Parse(v); err != nil {
			return errors.Wrap(err, "failed to parse PO_API_URL")
		} else {
			cfg.ApiUrl = strings.TrimSuffix(apiUrl.String(), "/")
		}
	}
	if v, ok := os.LookupEnv("PO_AUTH_TOKEN"); ok {
		cfg.Token = v
	}
	org, err := cmd.Flags().GetString("org")
	if err != nil {
		return errors.Wrap(err, "failed to get org flag")
	}
	if org != "" {
		cfg.DefaultOrg = org
	}

	if cfg.ApiUrl == "" {
		cfg.ApiUrl = defaultApiUrl
	}

	ctx := withConfiguration(cmd.Context(), cfg)
	ctx, err = withIamClient(ctx, cfg.ApiUrl)
	if err != nil {
		return errors.Wrap(err, "failed to setup iam client")
	}

	// Don't try and configure clients and context if this is a config command. Otherwise, we end up in a chicken
	// and egg situation.
	if FindCommandTreeAnnotation(cmd, SkipConfigContextAnnotation) == "" {
		ctx, err = withClients(ctx, cfg.ApiUrl, cfg.DefaultOrg, cfg.Token)
		if err != nil {
			cmd.SilenceUsage = true
			return errors.Wrap(err, "failed to setup clients")
		}
	}
	cmd.SetContext(ctx)

	versionCheckResult = internal.StartVersionCheck(ctx, internal.ModuleVersion, &cfg)

	return nil
}

func rootPersistentPostRun(cmd *cobra.Command, args []string) {
	if versionCheckResult != nil {
		if result := <-versionCheckResult; result != nil && result.NewVersionAvailable {
			result.DisplayNotification(cmd.ErrOrStderr())
		}
	}
}

var CrudGroup = &cobra.Group{ID: "crud", Title: "Verbs"}

func init() {
	RootCmd.Version = fmt.Sprintf("%s %s", internal.ModulePath, internal.ModuleVersion)
	RootCmd.PersistentFlags().BoolP("debug", "d", false, "Increase log verbosity to debug level")
	RootCmd.PersistentFlags().String("org", "", "Organization ID to use for all commands (overrides PO_ORG_ID environment variable)")
	RootCmd.AddGroup(CrudGroup)
	RootCmd.AddCommand(CreateCmd, GetCmd, UpdateCmd, DeleteCmd)
	RootCmd.AddCommand(ConfigCmd)
}
