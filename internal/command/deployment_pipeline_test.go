package command

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stellwerk-labs/platform-orchestrator-cli/clients/pipelines"
	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stretchr/testify/require"
)

const deploymentPipelineTestProject = "owned"

func TestPublishedDeploymentSelectionUsesOrdinaryContract(t *testing.T) {
	id := uuid.New()
	published := pipelines.PublishedPipelineVersion{Id: id, ProjectId: deploymentPipelineTestProject, Version: 7, Digest: "server-owned-digest"}
	selection, err := publishedDeploymentSelection(published, id, deploymentPipelineTestProject)
	require.NoError(t, err)
	body := dp.DeploymentCreateBody{ProjectId: deploymentPipelineTestProject, EnvId: "target", Mode: dp.DeploymentCreateBodyModeDeploy, Pipeline: selection}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, json.Unmarshal(raw, &document))
	require.Equal(t, map[string]any{"id": id.String(), "version": float64(7)}, document["pipeline"])
	// Captured Module tests wrap this same Deployment body; they must not replace its pipeline.
	wrapped, err := json.Marshal(dp.ModuleTestDeploymentCreateBody{Deployment: body, DraftTestSnapshots: map[string]uuid.UUID{uuid.NewString(): uuid.New()}})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(wrapped, &document))
	inner, ok := document["deployment"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{"id": id.String(), "version": float64(7)}, inner["pipeline"])
}
func TestPublishedDeploymentSelectionRejectsChangedIdentityOrRetirement(t *testing.T) {
	id := uuid.New()
	now := time.Now()
	for _, tc := range []struct {
		name  string
		value pipelines.PublishedPipelineVersion
	}{
		{"other version", pipelines.PublishedPipelineVersion{Id: uuid.New(), ProjectId: deploymentPipelineTestProject, Version: 1}},
		{"other project", pipelines.PublishedPipelineVersion{Id: id, ProjectId: "other", Version: 1}},
		{"invalid numeric version", pipelines.PublishedPipelineVersion{Id: id, ProjectId: deploymentPipelineTestProject}},
		{"retired", pipelines.PublishedPipelineVersion{Id: id, ProjectId: deploymentPipelineTestProject, Version: 1, RetiredAt: &now}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := publishedDeploymentSelection(tc.value, id, deploymentPipelineTestProject)
			require.Error(t, err)
		})
	}
}
func TestDeploymentPipelineFlagOnAllSharedEntrypoints(t *testing.T) {
	for _, cmd := range []*cobra.Command{DeployCmd, RollbackCmd, ScoreDeployCmd} {
		require.NotNil(t, cmd.Flags().Lookup(deployCmdPipelineVersionFlag), cmd.Name())
	}
	cmd := &cobra.Command{}
	cmd.Flags().String(deployCmdPipelineVersionFlag, "", "")
	selected, err := deploymentPipelineFromFlag(cmd, "org", deploymentPipelineTestProject)
	require.NoError(t, err)
	require.Nil(t, selected)
	for _, value := range []string{"", "invalid-selected-version", uuid.Nil.String()} {
		require.NoError(t, cmd.Flags().Set(deployCmdPipelineVersionFlag, value))
		_, err := deploymentPipelineFromFlag(cmd, "org", deploymentPipelineTestProject)
		require.Error(t, err)
	}
}
