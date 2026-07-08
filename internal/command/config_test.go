package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/config"
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
