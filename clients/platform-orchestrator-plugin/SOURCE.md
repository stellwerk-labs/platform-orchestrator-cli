# Experimental native plugin contract

This checked generated client is the minimal user-facing subset extracted from
the native Core OpenAPI worktree on 2026-09-28. It intentionally does not replace
the released `platform-orchestrator-cp` v2.1.0 client or its typed Rollouts DTOs.

- Core worktree HEAD: `2e80fd4b4a6127d132174e7bdb7b490fa8114529`
- source `openapi/spec.yaml` Git blob: `2fea52b621064c24fd6a85e616c03886fd8ce124`
- source file SHA-256: `b624314f46d35f1c164469614fa64b2a2e4b70cc4f8a14752bfcc82f3d431c50`
- extracted `spec.yaml` SHA-256: `7e3f80fb16c2b20daf90f59f6836ef7c68e233e9d7266de29e553e9b1ee22c23`

The extracted paths are plugin discovery, operation-bound invocation, and the
fixed protected Deployment material download. Regenerate exactly with:

```sh
go generate ./clients/platform-orchestrator-plugin
```

Repin this snapshot to a clean Core commit before release qualification.
