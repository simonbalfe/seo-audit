# SEO Audit guidance

SEO Audit has two Go executables: `seoaudit-api` owns the REST API, jobs, persistence, providers, and embedded read-only React dashboard; `seoaudit` is only a thin HTTP proxy and terminal formatter. It has separate public-site auditing, search-opportunity, backlink-analysis, and rank-tracking operations. Keep findings deterministic and tied to fetched evidence.

## Product contract

- Keep `seoaudit audit <url>` limited to public technical, on-page, and performance checks.
- Keep collection, provider credentials, validation, persistence, and analysis inside the API. CLI operation commands must remain HTTP clients and terminal formatters.
- Keep shared HTTP request and job wire types in `internal/protocol`; do not make the CLI import the API server or domain implementation packages.
- Keep API process startup in `cmd/seoaudit-api` and `internal/server`. Do not add server or dashboard startup back to the CLI package.
- Keep saved-evidence read models in `internal/evidence` and static dashboard hosting in `internal/webui`; do not mix either concern into the REST transport.
- Keep long crawls and provider operations as bounded asynchronous API jobs with cancellation, progress evidence, result retrieval, and idempotency protection.
- Keep authenticated and paid datasets in separate `opportunities`, `backlinks`, and `rankings` operations.
- Keep the public audit free of accounts and paid calls.
- Make paid SEO datasets explicit, report provider and request cost, and retain their source semantics rather than presenting estimates as first-party truth.
- Cache only complete paid-provider reports, keep refresh explicit, and distinguish current-call cost from the original cost of cached data.
- Store paid-provider snapshots locally with bounded retention. Do not make public audit job execution read or write SQLite unless persistence is explicitly requested.
- Keep the API and dashboard localhost-bound by default. Keep the dashboard UI read-only even though the API server also owns explicit data-collection endpoints. Audit and GSC persistence must remain explicit.
- Preserve rank checks as normalized keyword/device observations, including explicit not-ranking results and partial provider runs.
- Do not add AI-generated findings or citation monitoring without an explicit scope change.
- Keep requests bounded by page size, timeout, redirect count, sitemap count, crawl limit, and worker count.
- Do not describe heuristics as search-engine rules or ranking guarantees.
- Do not report missing optional schema as an error.
- Keep terminal output prioritised and concise while preserving full evidence in JSON.

## Change discipline

- Update `docs/coverage.md` when audit behaviour changes.
- Update `README.md` when flags, coverage, or observable output changes.
- Update `docs/api.md` when routes, request contracts, job semantics, or CLI-to-API mappings change.
- Run `go test ./...`, `go vet ./...`, and `go build ./cmd/seoaudit ./cmd/seoaudit-api` after substantive changes.
