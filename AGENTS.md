# SEO Audit guidance

SEO Audit is a Go CLI with a public-site audit by default and explicitly enabled external search data. Keep findings deterministic and tied to fetched evidence.

## Product contract

- Keep one user-facing operation: `seoaudit audit <url>`.
- Keep the default command free of accounts and paid calls.
- Make paid SEO datasets explicit, report provider and request cost, and retain their source semantics rather than presenting estimates as first-party truth.
- Do not add AI-generated findings or citation monitoring without an explicit scope change.
- Keep requests bounded by page size, timeout, redirect count, sitemap count, crawl limit, and worker count.
- Do not describe heuristics as search-engine rules or ranking guarantees.
- Do not report missing optional schema as an error.
- Keep terminal output prioritised and concise while preserving full evidence in JSON.

## Change discipline

- Update `docs/coverage.md` when audit behaviour changes.
- Update `README.md` when flags, coverage, or observable output changes.
- Run `go test ./...`, `go vet ./...`, and `go build ./cmd/seoaudit` after substantive changes.
