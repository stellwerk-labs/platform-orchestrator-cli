package command

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/config"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

func TestSetVersionCheck(t *testing.T) {
	testCases := []struct {
		name          string
		arg           string
		expectError   bool
		expectedValue bool
	}{
		{
			name:          "enable with enable",
			arg:           stringEnable,
			expectError:   false,
			expectedValue: false,
		},
		{
			name:          "enable with enabled",
			arg:           stringEnabled,
			expectError:   false,
			expectedValue: false,
		},
		{
			name:          "enable with yes",
			arg:           stringYes,
			expectError:   false,
			expectedValue: false,
		},
		{
			name:          "enable with on",
			arg:           stringOn,
			expectError:   false,
			expectedValue: false,
		},
		{
			name:          "enable with true",
			arg:           stringTrue,
			expectError:   false,
			expectedValue: false,
		},
		{
			name:          "disable with disable",
			arg:           stringDisable,
			expectError:   false,
			expectedValue: true,
		},
		{
			name:          "disable with disabled",
			arg:           stringDisabled,
			expectError:   false,
			expectedValue: true,
		},
		{
			name:          "disable with no",
			arg:           stringNo,
			expectError:   false,
			expectedValue: true,
		},
		{
			name:          "disable with off",
			arg:           stringOff,
			expectError:   false,
			expectedValue: true,
		},
		{
			name:          "disable with false",
			arg:           stringFalse,
			expectError:   false,
			expectedValue: true,
		},
		{
			name:          "case insensitive ENABLE",
			arg:           "ENABLE",
			expectError:   false,
			expectedValue: false,
		},
		{
			name:          "case insensitive DISABLE",
			arg:           "DISABLE",
			expectError:   false,
			expectedValue: true,
		},
		{
			name:        "invalid value",
			arg:         "invalid",
			expectError: true,
		},
		{
			name:        "invalid value maybe",
			arg:         "maybe",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			originalHome := os.Getenv("HOME")
			tmpHome := filepath.Join(tmpDir, "home")
			require.NoError(t, os.MkdirAll(filepath.Join(tmpHome, ".config", "octl"), 0750))
			require.NoError(t, os.Setenv("HOME", tmpHome))
			defer func() { _ = os.Setenv("HOME", originalHome) }()

			cmd := SetVersionCheck
			cmd.ResetCommands()
			cmd.ResetFlags()

			cmd.SetArgs([]string{tc.arg})

			err := cmd.Execute()

			if tc.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				cfg, err := config.ReadFile()
				require.NoError(t, err)
				require.NotNil(t, cfg.DisableVersionCheck)
				assert.Equal(t, tc.expectedValue, *cfg.DisableVersionCheck)
			}
		})
	}
}

const (
	setTokenTestCmd   = "set-token"
	setTokenTestToken = "set-token-test-value-0123456789"
	setTokenTestOld   = "previous-token"
	tokenFileFlag     = "--" + setTokenCmdTokenFileFlag
)

// withSetTokenHome points octl at a fresh configuration that already holds an
// organization, an API URL and a token, with version checks off.
func withSetTokenHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	require.NoError(t, config.SaveFile(config.Config{
		ApiUrl: "https://api.example.invalid", DefaultOrg: "set-token-org", Token: setTokenTestOld, DisableVersionCheck: ref.Ref(true),
	}))
}

// runSetToken runs 'octl config set-token' with stdin as standard input.
func runSetToken(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	RootCmd.SetIn(strings.NewReader(stdin))
	t.Cleanup(func() { RootCmd.SetIn(nil) })
	return executeAndResetCommand(context.Background(), RootCmd, append([]string{"config", setTokenTestCmd}, args...))
}

// requireStoredToken checks the stored token and that nothing else changed.
func requireStoredToken(t *testing.T, token string) {
	t.Helper()
	cfg, err := config.ReadFile()
	require.NoError(t, err)
	require.Equal(t, config.Config{
		ApiUrl: "https://api.example.invalid", DefaultOrg: "set-token-org", Token: token, DisableVersionCheck: ref.Ref(true),
	}, cfg)
}

func TestSetTokenReadsTheTokenFromAFileOrStandardInput(t *testing.T) {
	for name, input := range map[string]struct{ stdin, tokenFile string }{
		"file":                       {tokenFile: setTokenTestToken + "\n"},
		"file without line break":    {tokenFile: setTokenTestToken},
		"standard input":             {stdin: setTokenTestToken + "\n", tokenFile: "-"},
		"standard input with CRLF":   {stdin: setTokenTestToken + "\r\n", tokenFile: "-"},
		"standard input with spaces": {stdin: "  " + setTokenTestToken + "\t\n\n", tokenFile: "-"},
	} {
		t.Run(name, func(t *testing.T) {
			withSetTokenHome(t)
			tokenFile := input.tokenFile
			if tokenFile != "-" {
				tokenFile = writeTestFile(t, "octl.token", tokenFile)
			}
			stdout, stderr, err := runSetToken(t, input.stdin, tokenFileFlag, tokenFile)
			require.NoError(t, err)
			require.Equal(t, "Authentication token set successfully.\n", stderr, "no warning without an argument")
			require.NotContains(t, stdout+stderr, setTokenTestToken)
			requireStoredToken(t, setTokenTestToken)
		})
	}
}

func TestSetTokenRefusesInputThatIsNotOneToken(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.token")
	for name, tc := range map[string]struct {
		stdin string
		args  []string
		err   string
	}{
		"missing file":         {args: []string{tokenFileFlag, missing}, err: "failed to open the token file: open " + missing + ": no such file or directory"},
		"empty input":          {args: []string{tokenFileFlag, "-"}, err: "standard input must hold exactly one token, without spaces or line breaks inside it"},
		"only whitespace":      {stdin: " \n\t\n", args: []string{tokenFileFlag, "-"}, err: "standard input must hold exactly one token, without spaces or line breaks inside it"},
		"two lines":            {stdin: setTokenTestToken + "\n" + setTokenTestToken + "\n", args: []string{tokenFileFlag, "-"}, err: "standard input must hold exactly one token, without spaces or line breaks inside it"},
		"authorization header": {stdin: "Bearer " + setTokenTestToken, args: []string{tokenFileFlag, "-"}, err: "standard input must hold exactly one token, without spaces or line breaks inside it"},
		"oversized input":      {stdin: strings.Repeat(setTokenTestToken, maxTokenSize/len(setTokenTestToken)+1), args: []string{tokenFileFlag, "-"}, err: "standard input holds more than 65536 bytes, more than any token"},
		"file and argument":    {args: []string{tokenFileFlag, "-", setTokenTestToken}, err: "pass the token either with --token-file or as an argument, not both"},
		"no token":             {err: "pass the token with --token-file <file>, or with '--token-file -' from standard input"},
		"two arguments":        {args: []string{setTokenTestToken, setTokenTestToken}, err: "accepts at most 1 arg(s), received 2"},
	} {
		t.Run(name, func(t *testing.T) {
			withSetTokenHome(t)
			stdout, stderr, err := runSetToken(t, tc.stdin, tc.args...)
			require.EqualError(t, err, tc.err)
			require.NotContains(t, stdout+stderr+err.Error(), setTokenTestToken, "token input is never echoed")
			requireStoredToken(t, setTokenTestOld)
		})
	}
}

// Jay's decision: a token given as an argument keeps working, with a warning
// that names the safer input.
func TestSetTokenFromAnArgumentWarnsAboutShellHistory(t *testing.T) {
	withSetTokenHome(t)
	stdout, stderr, err := runSetToken(t, "", setTokenTestToken)
	require.NoError(t, err)
	require.Equal(t, "Warning: a token given as an argument ends up in shell history and the process list. "+
		"Pass it with '--token-file -' from standard input or with '--token-file <file>' instead.\n"+
		"Authentication token set successfully.\n", stderr)
	require.Empty(t, stdout)
	requireStoredToken(t, setTokenTestToken)
}
