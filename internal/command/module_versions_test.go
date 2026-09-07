package command

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedModuleVersionCommandsExposeConcurrencyAndAuditControls(t *testing.T) {
	for _, command := range []*cobra.Command{CreateManagedModuleVersion, UpdateManagedModuleVersion, CreateModuleVersionPin, CreateModuleVersionPinNote, UpdateModuleVersionPin} {
		assert.NotNil(t, command.Flag(moduleVersionIdempotencyFlag), command.Use)
	}
	assert.NotNil(t, UpdateManagedModuleVersion.Flag(moduleVersionExpectedFlag))
	assert.NotNil(t, UpdateManagedModuleVersion.Flag(moduleVersionReasonFlag))
	assert.NotNil(t, UpdateManagedModuleVersion.Flag(moduleVersionActionFlag))
	assert.NotNil(t, UpdateModuleVersionPin.Flag(moduleVersionExpectedFlag))
	assert.NotNil(t, CreateModuleVersionPinNote.Flag(moduleVersionNoteFlag))
}

func TestLegacyModuleWriteErrorExplainsMigrationWithoutInventingHistory(t *testing.T) {
	for _, message := range []string{"semantic_version is required"} {
		err := legacyModuleWriteError(message)
		require.ErrorContains(t, err, message)
		require.ErrorContains(t, err, "octl create module-catalogue-entry")
		require.ErrorContains(t, err, "--action promote")
		require.ErrorContains(t, err, "do not republish")
	}
	require.EqualError(t, legacyModuleWriteError("invalid id"), "request is invalid: invalid id")
	require.EqualError(t, legacyModuleWriteError("artifact_digest must be omitted for inline source"), "request is invalid: artifact_digest must be omitted for inline source")
}

func TestModulePinBulkPreviewAcceptsFrozenEnvironmentInput(t *testing.T) {
	for _, format := range []string{createUpdateCmdSetJsonFlag, createUpdateCmdSetYamlFlag} {
		t.Run(format, func(t *testing.T) {
			command := PreviewModuleVersionPinBulk
			require.NoError(t, command.ParseFlags([]string{"--" + format, "-"}))
			t.Cleanup(func() { require.NoError(t, command.Flags().Set(format, "")) })
			command.SetIn(strings.NewReader(`{"action":"pin","module_uuid":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","environment_uuids":["bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"]}`))
			body, err := readSetFlagsIntoType[cp.ModuleVersionPinBulkPreviewBody](command)
			require.NoError(t, err)
			require.Len(t, body.EnvironmentUuids, 1)
			assert.Equal(t, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", body.EnvironmentUuids[0].String())
			assert.Equal(t, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", body.ModuleUuid.String())
		})
	}
}

func TestModuleCommandIdempotencyKeyDefaultsToUUIDAndCanBeStable(t *testing.T) {
	command := *CreateManagedModuleVersion
	command.ResetFlags()
	command.Flags().String(moduleVersionIdempotencyFlag, "", "")

	generated := moduleCommandIdempotencyKey(&command)
	_, err := uuid.Parse(generated)
	require.NoError(t, err)
	require.NoError(t, command.Flags().Set(moduleVersionIdempotencyFlag, "release-video-demo"))
	assert.Equal(t, "release-video-demo", moduleCommandIdempotencyKey(&command))
}

func TestModuleConformanceAuthoringFieldsAndOptionalDigest(t *testing.T) {
	require.Contains(t, CreateResourceType.Long, "module_contract")
	require.Contains(t, CreateManagedModuleVersion.Long, "output_schema")
	command := &cobra.Command{}
	command.Flags().String(createUpdateCmdSetJsonFlag, "-", "")
	command.Flags().String(createUpdateCmdSetYamlFlag, "", "")
	command.Flags().StringArray(createUpdateCmdSetFlag, nil, "")
	command.SetIn(strings.NewReader(`{"module_source":"https://example.invalid/module.zip","source_revision":"0123456789abcdef","output_schema":{}}`))
	body, err := readSetFlagsIntoType[cp.ModuleVersionPublishBody](command)
	require.NoError(t, err)
	require.NotNil(t, body.OutputSchema)
	require.Empty(t, *body.OutputSchema)
	require.Nil(t, body.ArtifactDigest)
	command.SetIn(strings.NewReader(`{"output_schmea":{}}`))
	_, err = readSetFlagsIntoType[cp.ModuleVersionPublishBody](command)
	require.ErrorContains(t, err, "unknown field")
	command.SetIn(strings.NewReader(`{"module_contract":{"type":"object","required":["module_inputs"]},"output_schema":{}}`))
	resourceType, err := readSetFlagsIntoType[cp.ResourceTypeCreateBody](command)
	require.NoError(t, err)
	require.NotNil(t, resourceType.ModuleContract)
	require.Equal(t, "object", (*resourceType.ModuleContract)["type"])
}
