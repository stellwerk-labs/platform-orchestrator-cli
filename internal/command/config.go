package command

import (
	"io"
	"net/url"
	"os"
	"strings"
	"unicode"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/config"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
)

const (
	setTokenCmdTokenFileFlag = "token-file"

	// maxTokenSize bounds the token input; API tokens are far smaller.
	maxTokenSize = 64 << 10
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
	SetToken.Flags().String(setTokenCmdTokenFileFlag, "", "Read the token from this file, or from standard input with '-'")
	_ = SetToken.MarkFlagFilename(setTokenCmdTokenFileFlag)
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
	Use:   "set-token [token]",
	Args:  cobra.MaximumNArgs(1),
	Short: "Set the authentication token",
	Long: `Set the authentication token that octl sends to the API, for example the token of a service user.

Pass the token with --token-file, from a file or, with '-', from standard input. The input holds the token alone;
surrounding whitespace such as a final line break is ignored. A token given as an argument still works, but it ends up
in shell history and the process list, so octl warns about it. 'octl login' obtains a token in the browser instead, and
the PO_AUTH_TOKEN environment variable overrides the configured token.

$ octl config set-token --token-file service-user.token
$ octl config set-token --token-file -    # paste the token, then press Ctrl-D
`,
	Annotations: map[string]string{
		// avoid config reading errors when trying to set the config
		SkipConfigContextAnnotation: stringTrue,
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		var token string
		tokenFile, _ := cmd.Flags().GetString(setTokenCmdTokenFileFlag)
		switch {
		case len(args) == 1 && tokenFile != "":
			return errors.Errorf("pass the token either with --%s or as an argument, not both", setTokenCmdTokenFileFlag)
		case len(args) == 1:
			token = args[0]
			changedMessageF("Warning: a token given as an argument ends up in shell history and the process list. Pass it with '--%[1]s -' from standard input or with '--%[1]s <file>' instead.", setTokenCmdTokenFileFlag)
		case tokenFile != "":
			var err error
			if token, err = readToken(cmd.InOrStdin(), tokenFile); err != nil {
				return err
			}
		default:
			return errors.Errorf("pass the token with --%[1]s <file>, or with '--%[1]s -' from standard input", setTokenCmdTokenFileFlag)
		}

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

// readToken reads an API token from the file at path, or from stdin for "-".
// The input holds the token alone, up to surrounding whitespace. Errors never
// quote the input.
func readToken(stdin io.Reader, path string) (string, error) {
	source, in := "standard input", stdin
	if path != "-" {
		f, err := os.Open(path) //nolint:gosec // reading the token file the user names is the point
		if err != nil {
			return "", errors.Wrap(err, "failed to open the token file")
		}
		defer func() { _ = f.Close() }()
		source, in = path, f
	}
	raw, err := io.ReadAll(io.LimitReader(in, maxTokenSize+1))
	if err != nil {
		return "", errors.Wrapf(err, "failed to read the token from %s", source)
	}
	if len(raw) > maxTokenSize {
		return "", errors.Errorf("%s holds more than %d bytes, more than any token", source, maxTokenSize)
	}
	token := strings.TrimSpace(string(raw))
	if token == "" || strings.ContainsFunc(token, unicode.IsSpace) {
		return "", errors.Errorf("%s must hold exactly one token, without spaces or line breaks inside it", source)
	}
	return token, nil
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
