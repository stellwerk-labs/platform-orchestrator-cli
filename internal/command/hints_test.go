package command

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
)

func newTestCommand(short string, runE func(cmd *cobra.Command, args []string) error) *cobra.Command {
	return &cobra.Command{
		Use:   short,
		Short: short,
		RunE:  runE,
	}
}

func TestSuggestHintByCause_EnvNotFound_NoPrompt(t *testing.T) {
	cmd := newTestCommand("parent", nil)
	cmd.Flags().Bool(deployCmdNoPromptFlag, true, "")
	parentErr := errors.New("env not found")

	err := SuggestHintByCause(context.TODO(), HintCauseEnvNotFound, cmd, parentErr)
	if err != parentErr {
		t.Errorf("expected parentErr, got %v", err)
	}
}

func TestSuggestHintByCause_UnknownCause(t *testing.T) {
	cmd := newTestCommand("parent", nil)
	parentErr := errors.New("some error")

	err := SuggestHintByCause(context.TODO(), Cause(999), cmd, parentErr)
	if err != parentErr {
		t.Errorf("expected parentErr for unknown cause, got %v", err)
	}
}
