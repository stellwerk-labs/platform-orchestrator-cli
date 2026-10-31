package command

import (
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/internal"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/mcpserver"
)

var mcpCmd = &cobra.Command{
	Use: "mcp", Short: "Read-only Model Context Protocol integration", Args: cobra.NoArgs,
	Annotations: map[string]string{"mcp-stdio": "true"},
}
var mcpServeCmd = &cobra.Command{
	Use: "serve", SilenceUsage: true, Short: "Serve read-only metadata tools over stdio", Args: cobra.NoArgs,
	Long: "Serve read-only Stellwerk metadata tools over standard input/output. Uses the existing CLI configuration and authentication. Every tool requires an explicit org_id. No HTTP listener or interactive login is started.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		cfg, err := commandConfiguration(cmd)
		if err != nil {
			return errors.New("failed to read MCP configuration")
		}
		client, err := mcpserver.NewClient(cfg.ApiUrl, cfg.Token, os.Getenv(CAFileEnvVar))
		if err != nil {
			return err
		}
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		server := mcpserver.New(client, internal.ModuleVersion)
		if err := server.Run(ctx, &mcp.StdioTransport{MaxLineLength: 64 << 10}); err != nil && ctx.Err() == nil {
			return errors.New("MCP stdio session failed")
		}
		return nil
	},
}

func init() { mcpCmd.AddCommand(mcpServeCmd); RootCmd.AddCommand(mcpCmd) }
