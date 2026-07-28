# seo-audit

`seoaudit` is a Go CLI for deep technical and on-page SEO audits using only information published by the target website.

## Run an audit

```sh
seoaudit audit https://example.com
```

That single command discovers sitemaps, reads `robots.txt`, crawls the same domain, checks linked resources and external links, reconciles site-wide signals, and prints a prioritised action list.

Use `--json` for the complete crawl dataset and every affected URL:

```sh
seoaudit audit https://example.com --json > audit.json
```

The crawl is capped at 500 pages by default:

```sh
seoaudit audit https://example.com --limit 2000
```

## Coverage

- HTTP statuses, HTTPS, redirects, redirect chains, response type, size, and timing
- `robots.txt`, Googlebot access, search and AI crawler rules
- XML sitemap discovery, sitemap indexes, malformed sitemaps, and sitemap conflicts
- Indexability from response codes, robots directives, `X-Robots-Tag`, and canonicals
- HTML and HTTP canonicals, conflicting canonicals, and invalid canonical targets
- Titles and descriptions: missing, multiple, duplicate, short, and long
- H1 and heading structure: missing, multiple, duplicate, and skipped levels
- Exact and near-duplicate visible content
- Visible word count and very low-content indexable pages
- Internal-link graph, crawl depth, sitemap-only pages, broken links, redirecting links, empty anchors, and internal `nofollow`
- External link status checks
- Images: missing alt attributes, mixed HTTP assets, broken resources, and large files
- JSON-LD syntax and discovered schema types
- Hreflang targets and reciprocal return links
- Mobile viewport, page language, semantic main content, and mixed-content checks
- HTML page URL casing, underscores, parameters, and excessive length

## Boundaries

The audit reports public, observable evidence and labels heuristics as review items. It does not claim access to Google index state, rankings, traffic, keyword demand, backlinks, conversions, Core Web Vitals field data, or AI citations.

The crawler analyses server-returned HTML and automatically uses local Chrome for pages that expose very little raw content. Full raw-versus-rendered comparison on every page and browser performance traces are not yet included.

Deliberate `noindex` directives remain visible in page-level indexability data. They become actionable failures when a non-indexable URL is also submitted in a sitemap.

## Development

```sh
go test ./...
go vet ./...
go build ./cmd/seoaudit
```
