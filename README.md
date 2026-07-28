# seo-audit

`seoaudit` is a Go CLI for simple SEO and GEO checks using only information published by the target website.

Stage 1 does not require accounts, API keys, analytics access, paid SEO data, a database, or external SEO services.

## Stage 1

```sh
seoaudit audit https://example.com
seoaudit page https://example.com/about
seoaudit robots https://example.com
seoaudit sitemap https://example.com
seoaudit roadmap
```

Add `--json` for machine-readable output. By default, page reports show only failures and warnings. Add `--all` to include passing checks.

## What Stage 1 reads

- HTTP status, redirects, HTTPS, response type, and fetch time
- Page title, description, H1, canonical, robots directive, language, and viewport
- Internal and external links found on one page
- Images missing an `alt` attribute
- JSON-LD presence and JSON syntax
- Semantic main-content markup
- Search and AI crawler rules in `robots.txt`
- Sitemap declarations and public XML sitemap URLs

## Boundaries

The CLI reports observable facts and deterministic recommendations. It does not claim to know rankings, traffic, keyword demand, backlinks, conversions, or AI citations from the website alone.

Later stages are listed in [docs/roadmap.md](docs/roadmap.md). They remain unimplemented until the previous stage is useful and verified.

## Development

```sh
go test ./...
go vet ./...
go build ./cmd/seoaudit
```
