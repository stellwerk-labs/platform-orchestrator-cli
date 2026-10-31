package command

import (
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
)

func TestDraftInputRetainsExplicitContractAndOptionalDigest(t *testing.T) {
	command := modulePublicationCommand(t, `{"reason":"Try a new implementation","definition":{"semantic_version":"1.2.0","resource_type_version":2,"module_source":"inline","module_source_code":"output \"name\" { value = \"test\" }","module_params":{},"module_inputs":{},"provider_mapping":{},"dependencies":{},"coprovisioned":[],"output_schema":{}}}`)
	body, err := readSetFlagsIntoType[cp.ModuleDraftCreateBody](command)
	require.NoError(t, err)
	require.NotNil(t, body.Definition.ResourceTypeVersion)
	require.Equal(t, int64(2), *body.Definition.ResourceTypeVersion)
	require.Nil(t, body.Definition.ArtifactDigest)
	require.NotNil(t, body.Definition.OutputSchema)
	unknown := modulePublicationCommand(t, `{"reason":"Save","definition":{"semantic_version":"1.2.0","output_scehma":{}}}`)
	_, err = readSetFlagsIntoType[cp.ModuleDraftCreateBody](unknown)
	require.ErrorContains(t, err, "output_scehma")
}

func TestDraftStatusRejectsReplacementContentBeforeCallingServer(t *testing.T) {
	command := &cobra.Command{}
	command.Flags().String(createUpdateCmdSetJsonFlag, "", "")
	require.NoError(t, command.Flags().Set(createUpdateCmdSetJsonFlag, `{"definition":{"module_source":"other"}}`))
	err := UpdateModuleDraftStatus.RunE(command, []string{"database", uuid.NewString()})
	require.ErrorContains(t, err, "reviewed stored revision")
}

func TestAuthoringCommandsRejectInvalidIdentitiesBeforeCallingServer(t *testing.T) {
	err := GetResourceTypeVersion.RunE(&cobra.Command{}, []string{"database", "-1"})
	require.ErrorContains(t, err, "positive integer")
	err = UpdateModuleDraft.RunE(&cobra.Command{}, []string{"database", "not-a-uuid"})
	require.ErrorContains(t, err, "invalid Draft UUID")
}
