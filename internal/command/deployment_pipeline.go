package command

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stellwerk-labs/platform-orchestrator-cli/clients/pipelines"
	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
)

const deployCmdPipelineVersionFlag = "pipeline-version"

func deploymentPipelineFromFlag(cmd *cobra.Command, org, project string) (*dp.DeploymentPipelineSelection, error) {
	if cmd.Flags().Lookup(deployCmdPipelineVersionFlag) == nil {
		return nil, nil
	}
	value, _ := cmd.Flags().GetString(deployCmdPipelineVersionFlag)
	if value == "" && !cmd.Flags().Changed(deployCmdPipelineVersionFlag) {
		return nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil {
		return nil, fmt.Errorf("--pipeline-version must be a published version UUID")
	}
	target := pipelineCommandTarget{client: MustPipelineClient(cmd.Context()), org: org, project: project, id: id}
	published, err := target.readPublished(cmd)
	if err != nil {
		return nil, err
	}
	return publishedDeploymentSelection(*published, id, project)
}

func publishedDeploymentSelection(published pipelines.PublishedPipelineVersion, id uuid.UUID, project string) (*dp.DeploymentPipelineSelection, error) {
	if published.Id != id || published.ProjectId != project || published.Version < 1 {
		return nil, fmt.Errorf("published pipeline identity does not match the selected project/version")
	}
	if published.RetiredAt != nil {
		return nil, fmt.Errorf("selected pipeline version is retired; choose an allowed published version")
	}
	// The normal DP selector is {id,version}; plugin/Core {version_id,digest} is a different contract.
	return &dp.DeploymentPipelineSelection{Id: id.String(), Version: published.Version}, nil
}

func init() {
	for _, cmd := range []*cobra.Command{DeployCmd, RollbackCmd, ScoreDeployCmd} {
		cmd.Flags().String(deployCmdPipelineVersionFlag, "", "Exact published pipeline version UUID; omission uses the server's directional default")
	}
}
