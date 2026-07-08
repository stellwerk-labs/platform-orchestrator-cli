package command

import (
	"net/url"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/config"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
)

var ConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage the configuration",
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	},
}

func init() {
	ConfigCmd.AddCommand(SetOrg)
	ConfigCmd.AddCommand(SetUrl)
	ConfigCmd.AddCommand(SetToken)
	ConfigCmd.AddCommand(SetVersionCheck)
	ConfigCmd.AddCommand(ShowConfig)
	printer.SetupSingleOutputFormatFlag(ShowConfig.PersistentFlags())
}

var SetOrg = &cobra.Command{
	Use:   "set-org <org-id>",
	Args:  cobra.ExactArgs(1),
	Short: "Set the default organization on all commands unless overridden by environment variable or --org",
	Annotations: map[string]string{
		// avoid config reading errors when trying to set the config
		SkipConfigContextAnnotation: stringTrue,
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		org := args[0]

		cfg, err := config.ReadFile()
		if err != nil {
			return errors.Wrap(err, "failed to read config file")
		}

		cfg.DefaultOrg = org

		if err := config.SaveFile(cfg); err != nil {
			return errors.Wrap(err, "failed to save config file")
		}

		successMessageF("Organization set to %s.", org)
		return nil
	},
}

var SetUrl = &cobra.Command{
	Use:   "set-url <url>",
	Args:  cobra.ExactArgs(1),
	Short: "Set the API URL prefix (e.g. http://localhost:8080)",
	Annotations: map[string]string{
		// avoid config reading errors when trying to set the config
		SkipConfigContextAnnotation: stringTrue,
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		var apiUrl string
		if parsedUrl, err := url.Parse(args[0]); err != nil {
			return errors.Wrapf(err, "%s is not a valid url", args[0])
		} else {
			apiUrl = strings.TrimSuffix(parsedUrl.String(), "/")
		}

		cfg, err := config.ReadFile()
		if err != nil {
			return errors.Wrap(err, "failed to read config file")
		}

		cfg.ApiUrl = apiUrl

		if err := config.SaveFile(cfg); err != nil {
			return errors.Wrap(err, "failed to save config file")
		}

		successMessageF("API URL prefix set to %s.", apiUrl)
		return nil
	},
}

var ShowConfig = &cobra.Command{
	Use:     "show",
	Short:   "Show the current configuration",
	Aliases: []string{"get"},
	Annotations: map[string]string{
		// avoid config reading errors when trying to set the config
		SkipConfigContextAnnotation: stringTrue,
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		out, _ := cmd.Flags().GetString(printer.OutputFormatFlag)
		ctx, err := withPrinter(cmd.Context(), out, []string{printer.JsonPrinterType, printer.YamlPrinterType})
		if err != nil {
			return err
		}
		cmd.SetContext(ctx)
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cfg := MustConfiguration(cmd.Context())
		printer := MustPrinter(cmd.Context())
		return printer.Write(cmd.OutOrStdout(), cfg)
	},
}

var SetToken = &cobra.Command{
	Use:   "set-token <token>",
	Args:  cobra.ExactArgs(1),
	Short: "Set the authentication token",
	Annotations: map[string]string{
		// avoid config reading errors when trying to set the config
		SkipConfigContextAnnotation: stringTrue,
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		token := args[0]

		cfg, err := config.ReadFile()
		if err != nil {
			return errors.Wrap(err, "failed to read config file")
		}

		cfg.Token = token

		if err := config.SaveFile(cfg); err != nil {
			return errors.Wrap(err, "failed to save config file")
		}

		successMessageF("Authentication token set successfully.")
		return nil
	},
}

var SetVersionCheck = &cobra.Command{
	Use:   "set-version-check <enable|disable>",
	Args:  cobra.ExactArgs(1),
	Short: "Enable or disable automatic version checking",
	Long:  "Enable or disable automatic version checking. When enabled, octl will periodically check for new versions and notify you when updates are available. Disabled by default.",
	Annotations: map[string]string{
		// avoid config reading errors when trying to set the config
		SkipConfigContextAnnotation: stringTrue,
	},
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		var disableVersionCheck bool
		switch strings.ToLower(args[0]) {
		case stringEnable, stringEnabled, stringYes, stringOn, stringTrue:
			disableVersionCheck = false
		case stringDisable, stringDisabled, stringNo, stringOff, stringFalse:
			disableVersionCheck = true
		default:
			return errors.New("invalid value: must be 'enable' or 'disable'")
		}

		cfg, err := config.ReadFile()
		if err != nil {
			return errors.Wrap(err, "failed to read config file")
		}

		cfg.DisableVersionCheck = &disableVersionCheck

		if err := config.SaveFile(cfg); err != nil {
			return errors.Wrap(err, "failed to save config file")
		}

		if disableVersionCheck {
			successMessageF("Version checking disabled.")
		} else {
			successMessageF("Version checking enabled.")
		}
		return nil
	},
}
