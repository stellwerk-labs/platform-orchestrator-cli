package command

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	rollouts "github.com/stellwerk-labs/platform-orchestrator-cli/clients/progressive-rollouts"
)

func TestProgressiveRolloutInputPreservesDirectionalDefaultsAndSparseExceptions(t *testing.T) {
	for _, selection := range []string{
		`{"mode":"defaults"}`,
		`{"mode":"custom","overrides":[{"environment_uuid":"33333333-3333-4333-8333-333333333333","rollback":{"builtin":"default","digest":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}]}`,
	} {
		command := modulePublicationCommand(t, `{"pipeline_selection":`+selection+`}`)
		body, err := readSetFlagsIntoType[rollouts.ProgressiveRolloutSpec](command)
		require.NoError(t, err)
		require.NotNil(t, body.PipelineSelection)
		encoded, err := json.Marshal(body.PipelineSelection)
		require.NoError(t, err)
		require.JSONEq(t, selection, string(encoded))
	}
	legacy, err := readSetFlagsIntoType[rollouts.ProgressiveRolloutSpec](modulePublicationCommand(t, `{}`))
	require.NoError(t, err)
	require.Nil(t, legacy.PipelineSelection)
	encoded, err := json.Marshal(legacy)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "pipeline_selection")
	_, err = readSetFlagsIntoType[rollouts.ProgressiveRolloutSpec](modulePublicationCommand(t, `{"pipeline_selection":{"mode":"custom","override":[]}}`))
	require.ErrorContains(t, err, "override")
}
