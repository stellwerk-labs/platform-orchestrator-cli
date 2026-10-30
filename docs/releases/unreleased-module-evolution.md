# Orchestrator CLI Module authoring

Draft release notes, not a published release.

Adds commands for Draft authoring, exact stored-revision publication, discard, retained revisions and Resource Type interface history/publication. Planned SemVer remains editable until publication. New authoring commands require the matching server API; they do not execute a test by saving. The generated contract comes from the exact recorded server source. Server-side authority and concurrency checks remain authoritative.

Adds explicit immutable test capture/read commands and `deploy --draft-test` selection. Saving a new attempt leaves earlier captures and deployments unchanged. Test selection requires a matching server and never falls back to the ordinary deployment route.

Complete coordinated server/Console/client and execution qualification before
assigning a release version or publishing these artifacts. Existing immutable
published Versions and provider ownership must remain intact.
