# Read-only MCP server

`octl mcp serve` exposes Stellwerk metadata to an MCP client over standard
input/output. Configure and authenticate octl first using its normal configuration
commands or `PO_API_URL` and `PO_AUTH_TOKEN`. MCP always requires a configured
Bearer token, including local development. Use a token with the least read
permissions needed for your organization.

Add an entry to your MCP client's server configuration, using an absolute path to
your octl binary:

```json
{
  "mcpServers": {
    "stellwerk": {
      "command": "/absolute/path/to/octl",
      "args": ["mcp", "serve"]
    }
  }
}
```

The child process reads the same `~/.config/octl/config.yaml` as other commands.
The normal `PO_API_URL` and `PO_AUTH_TOKEN` environment overrides apply. Custom CA
bundles use `PO_CA_FILE`. Keep tokens in your existing protected configuration or
your client's secret environment mechanism. Avoid putting tokens in shared client
configuration, command arguments or chat messages.

Every tool call requires `org_id`, even when octl has a default organization.
Project, environment, module and version tools additionally require their explicit
scope IDs. MCP does not infer scope from conversation text or change the CLI
configuration. For example, call `list_environments` with:

```json
{"org_id":"my-org","project_id":"my-project","limit":25}
```

| Tool | Additional required arguments | Returned metadata |
| --- | --- | --- |
| `list_projects` | none | Project IDs, names, status and creation time |
| `get_project` | `project_id` | One project's metadata |
| `list_environments` | `project_id` | Environment IDs, names, type and status |
| `get_environment` | `project_id`, `environment_id` | One environment's metadata |
| `list_modules` | none | Active catalogue IDs, resource type, status and resource version |
| `list_module_versions` | `module_id` | Version identity, lifecycle, verification and interface version |
| `get_module_version` | `module_id`, `version_id` | One version's metadata |
| `get_resource_type` | `type_id` | Catalogue status, resource version, accessibility and output field count |

List calls accept `limit` (default 50, maximum 100). Paged API endpoints also accept
`page`, copied from `next_page_token` in the preceding result. `list_modules` uses
the unpaged catalogue endpoint and returns `truncated: true` if more entries exist
than the requested limit. It does not accept a page argument. IDs allow letters,
digits, dots, underscores and hyphens, start with a letter or digit, and have a
maximum length of 128. Traversal pairs (`..`), escapes, slashes and unknown
arguments are rejected.

The tool catalogue is fixed and read-only. No tool returns environment
configuration, labels, runner/provider configuration, module inputs or source,
state, credentials, encrypted outputs or decrypted output values. Resource Type
schema values and module contracts are excluded. Returned names are untrusted
operator-provided text. They are data, not instructions.

The server opens no HTTP listener, launches no browser or login prompt, and runs
no shell commands. It contacts only the configured API through generated clients.
HTTPS and normal certificate verification are required; explicit loopback HTTP
is supported for development with a configured token. Redirects are refused.
Developer-only extra request headers and separate DP/IAM endpoint overrides are
not used because these tools use only the configured control-plane API.

Standard output carries only SDK JSON-RPC frames, including with `--debug` or
`PO_CLI_DEBUG`. Startup errors go to standard error. MCP API errors expose a
sanitized failure or HTTP status, without server response bodies. There is no
startup version check. Each tool has a 30-second timeout, cancellation propagates
to HTTP, and at most four tool handlers run concurrently. Inbound stdio frames are
limited to 64 KiB, API bodies to 2 MiB and serialized metadata results to 128 KiB.
Oversized responses fail rather than returning partial objects. Closing standard
input or sending SIGINT/SIGTERM stops the process.

The implementation uses the official Go MCP SDK v1.8.0, with its typed tool
validation and stdio transport. Server-side authorization still controls which
organization metadata the configured token may read.
