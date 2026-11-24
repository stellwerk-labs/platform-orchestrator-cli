package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stellwerk-labs/platform-orchestrator-cli/clients/pipelines"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
	"gopkg.in/yaml.v3"
)

const pipelinePublishOperation = "publish"

func publishedPipelineCommand(cmd *cobra.Command, args []string, operation string) error {
	cmd.SilenceUsage = true
	org, err := ShouldOrg(cmd.Context())
	if err != nil {
		return err
	}
	id, err := uuid.Parse(args[1])
	if err != nil || id == uuid.Nil {
		return fmt.Errorf("pipeline identity must be a nonzero UUID")
	}
	target := pipelineCommandTarget{client: MustPipelineClient(cmd.Context()), org: org, project: args[0], id: id}
	switch operation {
	case pipelinePublishOperation:
		return target.publish(cmd)
	case "retire":
		return target.retire(cmd)
	default:
		version, err := target.readPublished(cmd)
		if err != nil {
			return err
		}
		return writePipelineOutput(cmd, version)
	}
}

func (target pipelineCommandTarget) publish(cmd *cobra.Command) error {
	revision, _ := cmd.Flags().GetInt64("expected-version")
	key, _ := cmd.Flags().GetString("request-key")
	if revision < 1 || len(key) > 128 || strings.TrimSpace(key) == "" {
		return fmt.Errorf("publication requires a positive --expected-version and stable --request-key (1-128 characters); reuse both when retrying")
	}
	res, err := target.client.PublishProjectPipelineWithResponse(cmd.Context(), target.org, target.project, target.id, pipelines.PipelinePublish{ExpectedResourceVersion: revision, RequestKey: key})
	if err != nil {
		return err
	}
	if err := pipelineStatus(res.StatusCode(), http.StatusOK); err != nil {
		return err
	}
	if res.JSON200 == nil {
		return fmt.Errorf("pipeline API returned no publication")
	}
	// Publication replay contains its original response, including historical retirement status.
	target.id = res.JSON200.Id
	current, err := target.readPublished(cmd)
	if err != nil {
		return err
	}
	if current.RetiredAt != nil {
		return fmt.Errorf("publication identifies a retired version; review a new draft revision and use a new request key")
	}
	return writePipelineOutput(cmd, current)
}

func (target pipelineCommandTarget) retire(cmd *cobra.Command) error {
	res, err := target.client.RetireProjectPipelineWithResponse(cmd.Context(), target.org, target.project, target.id)
	if err != nil {
		return err
	}
	if err := pipelineStatus(res.StatusCode(), http.StatusOK); err != nil {
		return err
	}
	if res.JSON200 == nil {
		return fmt.Errorf("pipeline API returned no retired version")
	}
	return writePipelineOutput(cmd, res.JSON200)
}

func (target pipelineCommandTarget) readPublished(cmd *cobra.Command) (*pipelines.PublishedPipelineVersion, error) {
	res, err := target.client.GetPublishedProjectPipelineWithResponse(cmd.Context(), target.org, target.project, target.id)
	if err != nil {
		return nil, err
	}
	if err := pipelineStatus(res.StatusCode(), http.StatusOK); err != nil {
		return nil, err
	}
	if res.JSON200 == nil {
		return nil, fmt.Errorf("pipeline API returned no published version")
	}
	return res.JSON200, nil
}

func pipelinePolicyDocument(cmd *cobra.Command) ([]byte, error) {
	raw, kind, err := readPipelineFile(cmd, 256*1024)
	if err != nil {
		return nil, err
	}
	if kind == "application/json" {
		return raw, nil
	}
	var value map[string]any
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("invalid pipeline policy YAML: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("pipeline policy must contain one YAML document")
	}
	result, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("invalid pipeline policy: %w", err)
	}
	return result, nil
}

func projectPipelinePolicyCommand(cmd *cobra.Command, args []string, write bool) error {
	cmd.SilenceUsage = true
	org, err := ShouldOrg(cmd.Context())
	if err != nil {
		return err
	}
	client := MustPipelineClient(cmd.Context())
	if !write {
		res, err := client.GetProjectPipelinePolicyWithResponse(cmd.Context(), org, args[0])
		if err != nil {
			return err
		}
		if err := pipelineStatus(res.StatusCode(), http.StatusOK); err != nil {
			return err
		}
		if res.JSON200 == nil {
			return fmt.Errorf("pipeline API returned no project policy")
		}
		return writePipelineOutput(cmd, res.JSON200)
	}
	raw, err := pipelinePolicyDocument(cmd)
	if err != nil {
		return err
	}
	res, err := client.SetProjectPipelinePolicyWithBodyWithResponse(cmd.Context(), org, args[0], "application/json", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	if err := pipelineStatus(res.StatusCode(), http.StatusOK); err != nil {
		return err
	}
	if res.JSON200 == nil {
		return fmt.Errorf("pipeline API returned no updated policy")
	}
	return writePipelineOutput(cmd, res.JSON200)
}

func environmentPipelineChoicesCommand(cmd *cobra.Command, args []string) error {
	org, err := ShouldOrg(cmd.Context())
	if err != nil {
		return err
	}
	res, err := MustPipelineClient(cmd.Context()).ListEnvironmentPipelineChoicesWithResponse(cmd.Context(), org, args[0], args[1])
	if err != nil {
		return err
	}
	if err := pipelineStatus(res.StatusCode(), http.StatusOK); err != nil {
		return err
	}
	if res.JSON200 == nil {
		return fmt.Errorf("pipeline API returned no environment choices")
	}
	return writePipelineOutput(cmd, res.JSON200)
}

func init() {
	for _, item := range []struct {
		parent                      *cobra.Command
		use, description, operation string
	}{
		{CreateCmd, "pipeline-version <project> <draft-uuid>", "Publish an exact reviewed draft revision without changing project policy", pipelinePublishOperation},
		{GetCmd, "pipeline-version <project> <version-uuid>", "Read an immutable published pipeline version", "get"},
		{DeleteCmd, "pipeline-version <project> <version-uuid>", "Retire a published version after removing policy references", "retire"},
	} {
		operation := item.operation
		cmd := &cobra.Command{Use: item.use, Short: item.description, Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error { return publishedPipelineCommand(cmd, args, operation) }}
		cmd.Flags().String(printer.OutputFormatFlag, "json", "Output format: json or yaml")
		if operation == pipelinePublishOperation {
			cmd.Flags().Int64("expected-version", 0, "Exact reviewed draft resource_version")
			cmd.Flags().String("request-key", "", "Stable publication request identity; reuse on retry")
		}
		item.parent.AddCommand(cmd)
	}
	for _, item := range []struct {
		parent *cobra.Command
		write  bool
	}{{GetCmd, false}, {UpdateCmd, true}} {
		write := item.write
		cmd := &cobra.Command{Use: "pipeline-policy <project>", Short: "Read or explicitly replace project pipeline policy with optimistic concurrency", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return projectPipelinePolicyCommand(cmd, args, write) }}
		cmd.Flags().String(printer.OutputFormatFlag, "json", "Output format: json or yaml")
		if write {
			cmd.Flags().StringP("file", "f", "", "JSON/YAML policy with expected_resource_version; omitted rollback_defaults preserves it, {} clears it")
		}
		item.parent.AddCommand(cmd)
	}
	choices := &cobra.Command{Use: "pipeline-choices <project> <environment>", Short: "Read compatible published pipelines and forward/rollback defaults", Args: cobra.ExactArgs(2), RunE: environmentPipelineChoicesCommand}
	choices.Flags().String(printer.OutputFormatFlag, "json", "Output format: json or yaml")
	GetCmd.AddCommand(choices)
}
