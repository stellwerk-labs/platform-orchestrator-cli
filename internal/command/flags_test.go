package command

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

const (
	flagsTestDeprecatedFlagName = "deprecated"
	flagsTestPrimaryFlagName    = "primary"
	flagsTestFlagName           = "test-flag"
)

func TestFlags_GetFlagWithFallback(t *testing.T) {
	tests := []struct {
		name           string
		setupFlags     func(*cobra.Command)
		flag           string
		deprecatedFlag string
		expected       string
	}{
		{
			name: "returns value from primary flag when set",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().String(flagsTestPrimaryFlagName, "default-primary", "primary flag")
				cmd.Flags().String(flagsTestDeprecatedFlagName, "default-deprecated", "deprecated flag")

				// Set flags
				_ = cmd.Flags().Set(flagsTestPrimaryFlagName, "primary-value")
			},
			flag:           flagsTestPrimaryFlagName,
			deprecatedFlag: flagsTestDeprecatedFlagName,
			expected:       "primary-value",
		},
		{
			name: "returns value from deprecated flag when primary not set",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().String(flagsTestPrimaryFlagName, "default-primary", "primary flag")
				cmd.Flags().String(flagsTestDeprecatedFlagName, "default-deprecated", "deprecated flag")

				// Set flags
				_ = cmd.Flags().Set(flagsTestDeprecatedFlagName, "deprecated-value")
			},
			flag:           flagsTestPrimaryFlagName,
			deprecatedFlag: flagsTestDeprecatedFlagName,
			expected:       "deprecated-value",
		},
		{
			name: "primary flag takes precedence over deprecated when both set",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().String(flagsTestPrimaryFlagName, "default-primary", "primary flag")
				cmd.Flags().String(flagsTestDeprecatedFlagName, "default-deprecated", "deprecated flag")

				// Set flags
				_ = cmd.Flags().Set(flagsTestPrimaryFlagName, "primary-value")
				_ = cmd.Flags().Set(flagsTestDeprecatedFlagName, "deprecated-value")
			},
			flag:           flagsTestPrimaryFlagName,
			deprecatedFlag: flagsTestDeprecatedFlagName,
			expected:       "primary-value",
		},
		{
			name: "returns default value from primary flag when neither set",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().String(flagsTestPrimaryFlagName, "default-primary", "primary flag")
				cmd.Flags().String(flagsTestDeprecatedFlagName, "default-deprecated", "deprecated flag")
			},
			flag:           flagsTestPrimaryFlagName,
			deprecatedFlag: flagsTestDeprecatedFlagName,
			expected:       "default-primary",
		},
		{
			name: "returns default value from deprecated flag when primary has no default",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().String(flagsTestPrimaryFlagName, "", "primary flag")
				cmd.Flags().String(flagsTestDeprecatedFlagName, "default-deprecated", "deprecated flag")
			},
			flag:           flagsTestPrimaryFlagName,
			deprecatedFlag: flagsTestDeprecatedFlagName,
			expected:       "default-deprecated",
		},
		{
			name: "returns empty string when no flags set and no defaults",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().String(flagsTestPrimaryFlagName, "", "primary flag")
				cmd.Flags().String(flagsTestDeprecatedFlagName, "", "deprecated flag")
			},
			flag:           flagsTestPrimaryFlagName,
			deprecatedFlag: flagsTestDeprecatedFlagName,
			expected:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			tt.setupFlags(cmd)

			result := GetFlagWithFallback(cmd, tt.flag, tt.deprecatedFlag)

			assert.Equal(t, tt.expected, result)

			if result != tt.expected {
				t.Errorf("Error: got '%v', expected '%v'", result, tt.expected)
			}
		})
	}
}

func TestFlags_getFlagValueIfSet(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*cobra.Command)
		flag     string
		expected string
	}{
		{
			name: "returns value when flag is set",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().String(flagsTestFlagName, "default", "test flag")

				// Set flags
				_ = cmd.Flags().Set(flagsTestFlagName, "test-value")
			},
			flag:     flagsTestFlagName,
			expected: "test-value",
		},
		{
			name: "returns empty string when flag not set even if there is a default value",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().String(flagsTestFlagName, "default", "test flag")
			},
			flag:     flagsTestFlagName,
			expected: "",
		},
		{
			name: "returns empty string when flag explicitly set to empty string",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().String(flagsTestFlagName, "default", "test flag")

				// Set flags
				_ = cmd.Flags().Set(flagsTestFlagName, "")
			},
			flag:     flagsTestFlagName,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			tt.setup(cmd)

			result := getFlagValueIfSet(cmd, tt.flag)

			assert.Equal(t, tt.expected, result)

			if result != tt.expected {
				t.Errorf("Error: got '%v', expected '%v'", result, tt.expected)
			}
		})
	}
}

func TestFlags_getDefaultFlagWithFallback(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(*cobra.Command)
		flag           string
		deprecatedFlag string
		expected       string
	}{
		{
			name: "returns primary flag default value if available",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().String(flagsTestPrimaryFlagName, "primary-default", "primary flag")
				cmd.Flags().String(flagsTestDeprecatedFlagName, "deprecated-default", "deprecated flag")
			},
			flag:           flagsTestPrimaryFlagName,
			deprecatedFlag: flagsTestDeprecatedFlagName,
			expected:       "primary-default",
		},
		{
			name: "returns deprecated flag default value when primary's unset",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().String(flagsTestPrimaryFlagName, "", "primary flag")
				cmd.Flags().String(flagsTestDeprecatedFlagName, "deprecated-default", "deprecated flag")
			},
			flag:           flagsTestPrimaryFlagName,
			deprecatedFlag: flagsTestDeprecatedFlagName,
			expected:       "deprecated-default",
		},
		{
			name: "returns empty string when defaults are not set",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().String(flagsTestPrimaryFlagName, "", "primary flag")
				cmd.Flags().String(flagsTestDeprecatedFlagName, "", "deprecated flag")
			},
			flag:           flagsTestPrimaryFlagName,
			deprecatedFlag: flagsTestDeprecatedFlagName,
			expected:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			tt.setup(cmd)

			result := getDefaultFlagWithFallback(cmd, tt.flag, tt.deprecatedFlag)

			assert.Equal(t, tt.expected, result)

			if result != tt.expected {
				t.Errorf(
					"Error: got '%v', want '%v'",
					result,
					tt.expected,
				)
			}
		})
	}
}
