package command

import (
	"github.com/spf13/cobra"
)

func GetFlagWithFallback(cmd *cobra.Command, flag, deprecatedFlag string) string {
	if v := getFlagValueIfSet(cmd, flag); v != "" {
		return v
	}

	if v := getFlagValueIfSet(cmd, deprecatedFlag); v != "" {
		return v
	}

	// If none of the flags are set, try to get a default value from either flag
	return getDefaultFlagWithFallback(cmd, flag, deprecatedFlag)
}

func getFlagValueIfSet(cmd *cobra.Command, flag string) string {
	if flagSet := cmd.Flags().Changed(flag); !flagSet {
		return ""
	}

	if v, _ := cmd.Flags().GetString(flag); v != "" {
		return v
	}

	return ""
}

func getDefaultFlagWithFallback(cmd *cobra.Command, flag, deprecatedFlag string) string {
	if v, _ := cmd.Flags().GetString(flag); v != "" {
		return v
	}

	if v, _ := cmd.Flags().GetString(deprecatedFlag); v != "" {
		return v
	}

	return ""
}
