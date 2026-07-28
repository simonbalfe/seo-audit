# SEO Audit guidance

SEO Audit is a public-site-only Go CLI. Keep findings deterministic and tied to evidence fetched from the target domain.

## Product contract

- Keep one user-facing operation: `seoaudit audit <url>`.
- Do not add private account access, paid SEO datasets, rankings, backlinks, keyword volume, or AI citation monitoring without an explicit scope change.
- Keep requests bounded by page size, timeout, redirect count, sitemap count, crawl limit, and worker count.
- Do not describe heuristics as search-engine rules or ranking guarantees.
- Do not report missing optional schema as an error.
- Keep terminal output prioritised and concise while preserving full evidence in JSON.

## Change discipline

- Update `docs/coverage.md` when audit behaviour changes.
- Update `README.md` when flags, coverage, or observable output changes.
- Run `go test ./...`, `go vet ./...`, and `go build ./cmd/seoaudit` after substantive changes.
