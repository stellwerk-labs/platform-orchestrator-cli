package command

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"slices"

	"github.com/pkg/browser"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

var ListActiveResourceNodes = &cobra.Command{
	Use:   "active-resource-nodes <project-id> <environment-id>",
	Args:  cobra.ExactArgs(2),
	Short: "List active resource graph nodes for a given environment",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		dpc := MustDpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		resp, err := dpc.ListActiveResourceNodesWithResponse(cmd.Context(), orgId, &dp.ListActiveResourceNodesParams{ProjectId: ref.Ref(args[0]), EnvId: ref.Ref(args[1])})
		if err != nil {
			return errors.Wrap(err, "failed to list active resource nodes")
		} else if resp.StatusCode() == http.StatusNotFound {
			return errors.New(resp.JSON404.Message)
		} else if resp.StatusCode() == http.StatusBadRequest {
			return errors.New(resp.JSON400.Message)
		} else if resp.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when listing active resource nodes: %s", resp.StatusCode(), string(resp.Body))
		}
		printer := MustPrinter(cmd.Context())
		return printer.Write(cmd.OutOrStdout(), resp.JSON200.Items)
	},
}

//go:embed active_resources_graph.html.tmpl
var activeResourceGraphTmpl string

var ShowActiveResourceGraph = &cobra.Command{
	Use:   "render-active-resource-graph <project-id> <environment-id>",
	Args:  cobra.ExactArgs(2),
	Short: "Render an HTML graph of the active resource graph for a given environment",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		dpc := MustDpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		resp, err := dpc.ListActiveResourceNodesWithResponse(cmd.Context(), orgId, &dp.ListActiveResourceNodesParams{ProjectId: ref.Ref(args[0]), EnvId: ref.Ref(args[1])})
		if err != nil {
			return errors.Wrap(err, "failed to list active resource nodes")
		} else if resp.StatusCode() == http.StatusNotFound {
			return errors.New(resp.JSON404.Message)
		} else if resp.StatusCode() == http.StatusBadRequest {
			return errors.New(resp.JSON400.Message)
		} else if resp.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when listing active resource nodes: %s", resp.StatusCode(), string(resp.Body))
		}

		nodes := slices.Collect(func(yield func(map[string]interface{}) bool) {
			for _, item := range resp.JSON200.Items {
				yield(map[string]interface{}{
					"id":         item.Id,
					"type":       item.ResourceType,
					"coordinate": fmt.Sprintf("%s.%s#%s", item.ResourceType, item.ResourceClass, item.ResourceId),
				})
			}
		})

		links := slices.Collect(func(yield func(map[string]interface{}) bool) {
			for _, item := range resp.JSON200.Items {
				for alias, target := range item.Edges {
					aliasType := "named"
					if raw, err := base64.RawStdEncoding.DecodeString(alias); err == nil && len(raw) == 32 {
						aliasType = "anonymous"
					}
					yield(map[string]interface{}{
						"source":    item.Id,
						"target":    target,
						"alias":     alias,
						"aliasType": aliasType,
					})
				}
			}
		})

		// NOTE: we only have one mode here - produce a d3 render graph as html and open it
		tmpl, err := template.New("").Funcs(map[string]any{
			"safeJson": func(v interface{}) template.JS {
				a, _ := json.Marshal(v)
				return template.JS(a) //nolint:gosec // we are intentionally injecting trusted JSON here
			},
		}).Parse(activeResourceGraphTmpl)
		if err != nil {
			return errors.Wrap(err, "failed to parse template")
		}
		buff := new(bytes.Buffer)
		if err := tmpl.Execute(buff, map[string]interface{}{
			"project_id": args[0],
			"env_id":     args[1],
			"nodes":      nodes,
			"links":      links,
		}); err != nil {
			return errors.Wrap(err, "failed to execute template")
		}

		outputFlag := GetFlagWithFallback(cmd, "result", "output")
		if outputFlag == "-" {
			_, err = cmd.OutOrStdout().Write(buff.Bytes())
			return err
		}

		var f *os.File
		if outputFlag == "" {
			if f, err = os.CreateTemp(os.TempDir(), "platform-orchestrator-active-resource-graph-*.html"); err != nil {
				return errors.Wrap(err, "failed to create temp file")
			}
		} else if f, err = os.Create(outputFlag); err != nil { //nolint:gosec // this is a CLI so this flag is controllable by the user for good reason.
			return errors.Wrap(err, "failed to open output file")
		}
		if _, err := f.Write(buff.Bytes()); err != nil {
			return errors.Wrap(err, "failed to write to output file")
		}
		_ = f.Close()

		_, _ = fmt.Fprintln(cmd.OutOrStdout(), f.Name())
		if openFlag, _ := cmd.Flags().GetBool("no-open"); !openFlag {
			return browser.OpenFile(f.Name())
		}
		return nil
	},
}

func init() {
	ShowActiveResourceGraph.Flags().Bool("no-open", false, "Do not open the graph in a browser")
	ShowActiveResourceGraph.Flags().String("result", "", "Output file to write the graph to, will be autogenerated if not specified, '-' indicates stdout")

	// Deprecated flag
	ShowActiveResourceGraph.Flags().String("output", "", "Output file to write the graph to, will be autogenerated if not specified, '-' indicates stdout")
	_ = ShowActiveResourceGraph.Flags().MarkDeprecated("output", "use --result instead")

	GetCmd.AddCommand(ListActiveResourceNodes)
	RootCmd.AddCommand(ShowActiveResourceGraph)
}
