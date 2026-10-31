# Unreleased: read-only MCP metadata

Adds `octl mcp serve`, a local stdio-only MCP server using the official Go SDK
v1.8.0. Eight allowlisted tools expose curated project, environment, module,
version and Resource Type metadata through the existing authenticated API.
Tools require explicit organization scope and exclude configuration, source,
state, credentials and output values. See [the MCP guide](../mcp.md).

CLI errors now consistently use standard error, preserving JSON-RPC-only
standard output for MCP clients.

Security: pins the effective transitive Brotli build/tool module graph to v1.2.7
to address GHSA-c9gr-gmg4-2vqv. Brotli is absent from the CLI executable and runtime
package dependencies.
