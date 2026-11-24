package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stellwerk-labs/platform-orchestrator-cli/clients/pipelines"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/config"
	"github.com/stretchr/testify/require"
)

func TestPipelinePolicyDocument(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		valid       bool
	}{
		{"JSON preserves server duplicate-key validation", `{"enabled":true,"enabled":false,"allowed_version_ids":[],"rollback_defaults":{},"expected_resource_version":0}`, true},
		{"YAML directional clear", "enabled: true\nallowed_version_ids: []\nrollback_defaults: {}\nexpected_resource_version: 0\n", true},
		{"YAML duplicate", "enabled: true\nenabled: false\n", false},
		{"YAML second document", "enabled: true\n---\nenabled: false\n", false},
		{"YAML malformed suffix", "enabled: true\n---\n[", false},
		{"bounded policy", strings.Repeat("x", 256*1024+1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("file", "-", "")
			cmd.SetIn(strings.NewReader(tc.input))
			raw, err := pipelinePolicyDocument(cmd)
			if !tc.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.True(t, json.Valid(raw))
			if strings.HasPrefix(tc.input, "{") {
				require.Equal(t, tc.input, string(raw))
			}
		})
	}
}

func TestPipelinePublishedLiveLifecycle(t *testing.T) {
	project := os.Getenv("PIPELINE_TEST_PROJECT")
	if project == "" {
		t.Skip("PIPELINE_TEST_PROJECT must identify an isolated acceptance project")
	}
	ctx, err := withClients(t.Context(), os.Getenv("PO_API_URL"), os.Getenv("PO_ORG_ID"), os.Getenv("PO_AUTH_TOKEN"))
	require.NoError(t, err)
	ctx = withConfiguration(ctx, config.Config{DefaultOrg: os.Getenv("PO_ORG_ID")})
	command := func() *cobra.Command {
		cmd := &cobra.Command{}
		cmd.SetContext(ctx)
		cmd.Flags().String("out", "json", "")
		cmd.Flags().String("file", "-", "")
		cmd.Flags().Int64("expected-version", 1, "")
		cmd.Flags().String("request-key", "cli-"+uuid.NewString(), "")
		cmd.SetOut(&bytes.Buffer{})
		return cmd
	}
	cmd := command()
	cmd.SetIn(strings.NewReader(fmt.Sprintf(`{"slug":"cli-published-%s","name":"CLI publication","definition":{"strategy":"apply","steps":[]}}`, uuid.NewString()[:8])))
	require.NoError(t, runPipelineCommand(cmd, []string{project}, pipelineCreate))
	var draft pipelines.PipelineDraft
	require.NoError(t, json.Unmarshal(cmd.OutOrStdout().(*bytes.Buffer).Bytes(), &draft))
	publish := command()
	require.NoError(t, publishedPipelineCommand(publish, []string{project, draft.Id.String()}, "publish"))
	var version pipelines.PublishedPipelineVersion
	require.NoError(t, json.Unmarshal(publish.OutOrStdout().(*bytes.Buffer).Bytes(), &version))
	retry := command()
	key, _ := publish.Flags().GetString("request-key")
	require.NoError(t, retry.Flags().Set("request-key", key))
	require.NoError(t, publishedPipelineCommand(retry, []string{project, draft.Id.String()}, "publish"))
	var replay pipelines.PublishedPipelineVersion
	require.NoError(t, json.Unmarshal(retry.OutOrStdout().(*bytes.Buffer).Bytes(), &replay))
	require.Equal(t, version.Id, replay.Id)
	get := command()
	require.NoError(t, projectPipelinePolicyCommand(get, []string{project}, false))
	var policy pipelines.PipelinePolicy
	require.NoError(t, json.Unmarshal(get.OutOrStdout().(*bytes.Buffer).Bytes(), &policy))
	require.False(t, policy.Enabled)
	require.Empty(t, policy.AllowedVersionIds)
	set := command()
	set.SetIn(strings.NewReader(fmt.Sprintf("expected_resource_version: %d\nenabled: true\nallowed_version_ids: [%s]\ndefault_version_id: %s\nrollback_defaults:\n  default_version_id: %s\n", policy.ResourceVersion, version.Id, version.Id, version.Id)))
	require.NoError(t, projectPipelinePolicyCommand(set, []string{project}, true))
	require.NoError(t, json.Unmarshal(set.OutOrStdout().(*bytes.Buffer).Bytes(), &policy))
	retire := command()
	require.Error(t, publishedPipelineCommand(retire, []string{project, version.Id.String()}, "retire"))
	clear := command()
	clear.SetIn(strings.NewReader(fmt.Sprintf(`{"expected_resource_version":%d,"enabled":false,"allowed_version_ids":[],"rollback_defaults":{}}`, policy.ResourceVersion)))
	require.NoError(t, projectPipelinePolicyCommand(clear, []string{project}, true))
	require.NoError(t, publishedPipelineCommand(command(), []string{project, version.Id.String()}, "retire"))
	archive := command()
	require.NoError(t, runPipelineCommand(archive, []string{project, draft.Id.String()}, pipelineDelete))
}
