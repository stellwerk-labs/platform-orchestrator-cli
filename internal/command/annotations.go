package command

import "github.com/spf13/cobra"

// SkipConfigContextAnnotation skips the pre-run hook that reads the config file and adds it into the command context.
const SkipConfigContextAnnotation = "skip-config-context"

// FindCommandTreeAnnotation returns the first value set for the given annotation, or returns an empty string.
func FindCommandTreeAnnotation(cmd *cobra.Command, k string) string {
	for {
		if v, ok := cmd.Annotations[k]; ok {
			return v
		} else if p := cmd.Parent(); p != nil {
			cmd = p
		} else {
			return ""
		}
	}
}
