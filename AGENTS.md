# SEO Audit guidance

SEO Audit is a public-site-only Go CLI. Keep findings deterministic and tied to evidence fetched from the target domain.

## Scope

- Implement one roadmap stage at a time.
- Do not add private account access, paid SEO datasets, rank tracking, backlinks, keyword volume, or AI citation monitoring without an explicit scope change.
- Keep network requests bounded by page size, timeout, redirect count, sitemap count, and later crawl limits.
- Do not describe heuristics as search-engine rules or ranking guarantees.
- Keep human output concise and JSON output stable.

## Change discipline

- Update `docs/roadmap.md` when stage scope or status changes.
- Update `README.md` when commands or observable output change.
- Run `go test ./...`, `go vet ./...`, and `go build ./cmd/seoaudit` after substantive changes.
