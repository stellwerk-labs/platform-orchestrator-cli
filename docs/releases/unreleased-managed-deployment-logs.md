# Orchestrator CLI managed deployment logs

Draft release notes, not a published release.

`octl deploy`, `octl score deploy` and `octl rollback` now request managed runner
log encryption by default. `octl logs <deployment-id>` reads those logs using the
authenticated account, requiring `deployment_logs_read`. `--show-logs` uses the
same authenticated retrieval after deployment completes.

`--skip-logs` disables collection. `--print-logs-key` explicitly selects a
caller-owned age key and prints its existing exposure warning. Historical
caller-owned logs still require `octl logs --key-file <file>` (or `--key-file -`
for standard input). `octl logs --encrypted` downloads the original encrypted
bytes. Managed retrieval never falls back to caller-owned encryption or key
printing. Deployment output encryption is unchanged.

These commands require the coordinated Data Plane managed-logs API and IAM
permission update. The recorded API source is currently local and unpublished;
complete public-source and coordinated release checks before publication.
