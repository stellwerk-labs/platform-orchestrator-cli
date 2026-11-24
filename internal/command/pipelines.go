package command

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stellwerk-labs/platform-orchestrator-cli/clients/pipelines"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
)

const (
	pipelineDocumentLimit = 32 * 1024
	pipelineCreate        = "create"
	pipelineGet           = "get"
	pipelineList          = "list"
	pipelineUpdate        = "update"
	pipelineDelete        = "delete"
	pipelineValidate      = "validate"
	pipelineIdentityUse   = "pipeline <project> <pipeline-uuid>"
)

func readPipelineDocument(cmd *cobra.Command) ([]byte, string, error) {
	return readPipelineFile(cmd, pipelineDocumentLimit)
}

func readPipelineFile(cmd *cobra.Command, limit int) ([]byte, string, error) {
	filename, _ := cmd.Flags().GetString("file")
	if filename == "" {
		return nil, "", fmt.Errorf("--file is required; use - for stdin")
	}
	reader := cmd.InOrStdin()
	if filename != "-" {
		file, err := os.Open(filename) // #nosec G304 -- Reading a user-selected document path is the purpose of --file.
		if err != nil {
			return nil, "", fmt.Errorf("open pipeline document: %w", err)
		}
		defer func() { _ = file.Close() }()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
	if err != nil {
		return nil, "", fmt.Errorf("read pipeline document: %w", err)
	}
	if len(raw) > limit {
		return nil, "", fmt.Errorf("pipeline document exceeds %d bytes", limit)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, "", fmt.Errorf("pipeline document is empty")
	}
	contentType := "application/yaml"
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		contentType = "application/json"
	}
	return raw, contentType, nil
}

func pipelineRevision(cmd *cobra.Command) (string, error) {
	version, _ := cmd.Flags().GetInt64("expected-version")
	if version < 1 {
		return "", fmt.Errorf("--expected-version must be the positive resource_version from get pipeline --details")
	}
	return strconv.Quote(strconv.FormatInt(version, 10)), nil
}

func pipelineStatus(status, expected int) error {
	if status == expected {
		return nil
	}
	switch status {
	case http.StatusConflict:
		return fmt.Errorf("pipeline conflict: re-read the reviewed revision or policy before retrying; retirement requires removing policy references and draft deletion requires retiring published versions; create conflicts require an explicit existing ID")
	case http.StatusForbidden:
		return fmt.Errorf("pipeline operation forbidden; check project pipeline permissions")
	case http.StatusNotFound:
		return fmt.Errorf("pipeline or project not found")
	default:
		return fmt.Errorf("pipeline API returned HTTP %d", status)
	}
}

func writePipelineOutput(cmd *cobra.Command, value any) error {
	format, _ := cmd.Flags().GetString(printer.OutputFormatFlag)
	switch format {
	case printer.JsonPrinterType:
		return (&printer.JsonPrinter{}).Write(cmd.OutOrStdout(), value)
	case printer.YamlPrinterType:
		return (&printer.YamlPrinter{}).Write(cmd.OutOrStdout(), value)
	default:
		return fmt.Errorf("pipeline document output requires --out json or --out yaml")
	}
}

type pipelineCommandTarget struct {
	client       pipelines.ClientWithResponsesInterface
	org, project string
	id           uuid.UUID
}

func runPipelineCommand(cmd *cobra.Command, args []string, operation string) error {
	cmd.SilenceUsage = true
	org, err := ShouldOrg(cmd.Context())
	if err != nil {
		return err
	}
	target := pipelineCommandTarget{client: MustPipelineClient(cmd.Context()), org: org, project: args[0]}
	if len(args) == 2 {
		target.id, err = uuid.Parse(args[1]) // #nosec G602 -- The enclosing len(args) == 2 check proves index 1 is in bounds.
		if err != nil {
			return fmt.Errorf("invalid pipeline UUID: %w", err)
		}
	}
	switch operation {
	case pipelineCreate:
		return target.create(cmd)
	case pipelineUpdate:
		return target.update(cmd)
	case pipelineDelete:
		return target.delete(cmd)
	case pipelineValidate:
		return target.validate(cmd)
	case pipelineGet:
		return target.get(cmd)
	case pipelineList:
		return target.list(cmd)
	default:
		return fmt.Errorf("unsupported pipeline operation %q", operation)
	}
}

func (target pipelineCommandTarget) create(cmd *cobra.Command) error {
	raw, contentType, err := readPipelineDocument(cmd)
	if err != nil {
		return err
	}
	res, err := target.client.CreateProjectPipelineWithBodyWithResponse(cmd.Context(), target.org, target.project, contentType, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	if err := pipelineStatus(res.StatusCode(), 201); err != nil {
		return err
	}
	if res.JSON201 == nil {
		return fmt.Errorf("pipeline API returned no created draft")
	}
	return writePipelineOutput(cmd, res.JSON201)
}

func (target pipelineCommandTarget) update(cmd *cobra.Command) error {
	raw, contentType, err := readPipelineDocument(cmd)
	if err != nil {
		return err
	}
	revision, err := pipelineRevision(cmd)
	if err != nil {
		return err
	}
	res, err := target.client.SaveProjectPipelineDocumentWithBodyWithResponse(cmd.Context(), target.org, target.project, target.id, &pipelines.SaveProjectPipelineDocumentParams{IfMatch: revision}, contentType, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	if err := pipelineStatus(res.StatusCode(), 200); err != nil {
		return err
	}
	if res.JSON200 == nil {
		return fmt.Errorf("pipeline API returned no updated draft")
	}
	return writePipelineOutput(cmd, res.JSON200)
}

func (target pipelineCommandTarget) delete(cmd *cobra.Command) error {
	revision, err := pipelineRevision(cmd)
	if err != nil {
		return err
	}
	res, err := target.client.DeleteProjectPipelineDraftWithResponse(cmd.Context(), target.org, target.project, target.id, &pipelines.DeleteProjectPipelineDraftParams{IfMatch: revision})
	if err != nil {
		return err
	}
	return pipelineStatus(res.StatusCode(), 204)
}

func (target pipelineCommandTarget) validate(cmd *cobra.Command) error {
	raw, contentType, err := readPipelineDocument(cmd)
	if err != nil {
		return err
	}
	res, err := target.client.ValidateProjectPipelineDocumentWithBodyWithResponse(cmd.Context(), target.org, target.project, contentType, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	if err := pipelineStatus(res.StatusCode(), 200); err != nil {
		return err
	}
	if res.JSON200 == nil {
		return fmt.Errorf("pipeline API returned no validation result")
	}
	if err := writePipelineOutput(cmd, res.JSON200); err != nil {
		return err
	}
	if !res.JSON200.Valid {
		return fmt.Errorf("pipeline document is invalid")
	}
	return nil
}

func (target pipelineCommandTarget) get(cmd *cobra.Command) error {
	details, _ := cmd.Flags().GetBool("details")
	if details {
		return target.getDetails(cmd)
	}
	res, err := target.client.GetProjectPipelineDocumentWithResponse(cmd.Context(), target.org, target.project, target.id)
	if err != nil {
		return err
	}
	if err := pipelineStatus(res.StatusCode(), 200); err != nil {
		return err
	}
	if res.JSON200 == nil {
		return fmt.Errorf("pipeline API returned no document")
	}
	return writePipelineOutput(cmd, res.JSON200)
}

func (target pipelineCommandTarget) getDetails(cmd *cobra.Command) error {
	res, err := target.client.GetProjectPipelineDraftWithResponse(cmd.Context(), target.org, target.project, target.id)
	if err != nil {
		return err
	}
	if err := pipelineStatus(res.StatusCode(), 200); err != nil {
		return err
	}
	if res.JSON200 == nil {
		return fmt.Errorf("pipeline API returned no draft")
	}
	return writePipelineOutput(cmd, res.JSON200)
}

func (target pipelineCommandTarget) list(cmd *cobra.Command) error {
	res, err := target.client.ListProjectPipelinesWithResponse(cmd.Context(), target.org, target.project)
	if err != nil {
		return err
	}
	if err := pipelineStatus(res.StatusCode(), 200); err != nil {
		return err
	}
	if res.JSON200 == nil {
		return fmt.Errorf("pipeline API returned no catalogue")
	}
	format, _ := cmd.Flags().GetString(printer.OutputFormatFlag)
	if format == printer.TablePrinterType {
		return writePipelineTable(cmd.OutOrStdout(), res.JSON200.Drafts)
	}
	return writePipelineOutput(cmd, res.JSON200)
}

func writePipelineTable(output io.Writer, drafts []pipelines.PipelineDraft) error {
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "ID\tSLUG\tNAME\tREVISION"); err != nil {
		return err
	}
	for _, draft := range drafts {
		name := strings.Map(pipelineTableRune, draft.Name)
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%d\n", draft.Id, draft.Slug, name, draft.ResourceVersion); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func pipelineTableRune(r rune) rune {
	if r < 32 || r == 127 {
		return ' '
	}
	return r
}

func init() {
	for _, item := range []struct {
		parent                      *cobra.Command
		operation, use, description string
		args                        int
	}{
		{CreateCmd, pipelineCreate, "pipeline <project>", "Create a pipeline draft from JSON or YAML (does not publish)", 1},
		{GetCmd, pipelineGet, pipelineIdentityUse, "Export a canonical pipeline document", 2},
		{GetCmd, pipelineList, "pipelines <project>", "List project pipeline drafts", 1},
		{UpdateCmd, pipelineUpdate, pipelineIdentityUse, "Update a draft using its reviewed revision", 2},
		{DeleteCmd, pipelineDelete, pipelineIdentityUse, "Archive an unused draft, preserving execution history", 2},
	} {
		operation := item.operation
		cmd := &cobra.Command{Use: item.use, Short: item.description, Args: cobra.ExactArgs(item.args), RunE: func(cmd *cobra.Command, args []string) error { return runPipelineCommand(cmd, args, operation) }}
		configurePipelineFlags(cmd, operation)
		item.parent.AddCommand(cmd)
	}
	validate := &cobra.Command{Use: pipelineValidate, Short: "Validate declarative documents", GroupID: CrudGroup.ID}
	cmd := &cobra.Command{Use: "pipeline <project>", Short: "Validate JSON or YAML with the server schema and domain rules", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return runPipelineCommand(cmd, args, pipelineValidate) }}
	configurePipelineFlags(cmd, pipelineValidate)
	validate.AddCommand(cmd)
	RootCmd.AddCommand(validate)
}

func configurePipelineFlags(cmd *cobra.Command, operation string) {
	if operation == pipelineCreate || operation == pipelineUpdate || operation == pipelineValidate {
		cmd.Flags().StringP("file", "f", "", "JSON/YAML document path, or - for stdin")
	}
	if operation == pipelineUpdate || operation == pipelineDelete {
		cmd.Flags().Int64("expected-version", 0, "Exact draft resource_version reviewed before this change")
	}
	if operation == pipelineGet {
		cmd.Flags().Bool("details", false, "Include server identity and resource_version instead of exporting the document")
	}
	if operation != pipelineDelete && operation != pipelineList {
		cmd.Flags().String(printer.OutputFormatFlag, "json", "Output format: json or yaml")
	}
}
