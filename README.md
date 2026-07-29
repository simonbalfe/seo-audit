# seo-audit

SEO Audit ships as two cleanly separated Go programs:

- `seoaudit-api` runs the REST API, jobs, provider integrations, SQLite persistence, and embedded read-only dashboard;
- `seoaudit` is a thin HTTP proxy that exposes `audit`, `opportunities`, `backlinks`, and `rankings` as terminal commands.

Account data and paid calls never run as part of `audit`.

## Start the API

Run the localhost API before using the proxy commands:

```sh
seoaudit-api
```

The API listens at `http://127.0.0.1:8787` and serves the dashboard from the same address. In another terminal:

```sh
seoaudit audit https://example.com
```

The CLI submits the request, follows the asynchronous job, retrieves the complete result, and formats it for the terminal. Crawling, credentials, paid calls, caching, persistence, and analysis all remain inside the API process.

Use another API address with `SEOAUDIT_API_URL` or the root `--api-url` flag:

```sh
export SEOAUDIT_API_URL="http://127.0.0.1:8787"
seoaudit audit https://example.com
```

See [`docs/api.md`](docs/api.md) for every endpoint, request body, job state, error response, and CLI mapping.

## Project structure

```text
cmd/
├── seoaudit/       thin CLI proxy executable
└── seoaudit-api/   API and dashboard server executable
dashboard/          React application and embedded production assets
internal/
├── api/            REST transport, orchestration, and jobs
├── apiclient/      reusable HTTP client used by the CLI
├── cli/            Cobra commands and terminal formatting
├── evidence/       saved-evidence read model used by site endpoints
├── protocol/       shared HTTP request and job wire types
├── server/         process configuration and API/UI composition
├── webui/          static dashboard hosting only
├── audit/          public crawler and deterministic analysis
├── dataforseo/     paid provider adapter
├── gsc/            Search Console adapter
├── ranktracking/   ranking domain logic
└── storage/        SQLite persistence
```

The CLI depends only on `internal/apiclient` and `internal/protocol`; it does not import the API server, crawler, providers, persistence, or dashboard. The REST handler does not serve static assets. `internal/server` is the single composition point.

## Run an audit

```sh
seoaudit audit https://example.com
```

That single command discovers sitemaps, reads `robots.txt`, crawls the same domain, checks linked resources and external links, runs mobile performance tests on representative page templates, reconciles site-wide signals, and prints a prioritised action list.

Use `--json` for the complete crawl dataset and every affected URL:

```sh
seoaudit audit https://example.com --json > audit.json
```

Use `--verbose` or `-v` to stream API job progress for robots, sitemap, page crawl, analysis, and resource checks to stderr:

```sh
seoaudit audit https://example.com --verbose
```

Verbose logs can be combined with JSON without corrupting the JSON written to stdout:

```sh
seoaudit audit https://example.com --verbose --json > audit.json
```

The crawl is capped at 500 pages by default:

```sh
seoaudit audit https://example.com --limit 2000
```

Performance testing is enabled by default. It uses local Chrome to test the homepage and up to five representative indexable sections under a bounded simulated mobile profile:

```sh
seoaudit audit https://example.com
```

Disable the slower browser performance stage when you only need the HTTP and HTML crawl:

```sh
seoaudit audit https://example.com --performance=false
```

Ask the API to save a completed audit to its dashboard database:

```sh
seoaudit audit https://example.com --save
```

Without `--save`, the public audit does not write an audit snapshot.

## Find search opportunities

Read first-party Google Search Console query and page performance:

```sh
seoaudit opportunities https://example.com \
  --gsc \
  --gsc-site "sc-domain:example.com"
```

The integration makes one read-only Search Analytics API request for finalized query/page rows. It reports returned clicks, impressions, CTR, weighted position, queries in positions 4–20, and queries observed against multiple pages. The default period is 28 days and the returned dataset is capped at 250 rows:

```sh
seoaudit opportunities https://example.com --gsc --gsc-days 90 --gsc-limit 1000
```

Add `--save` to ask the API to retain the returned Search Console report for the dashboard:

```sh
seoaudit opportunities https://example.com --gsc --save
```

`--gsc-site` accepts a domain property such as `sc-domain:example.com` or a URL-prefix property such as `https://www.example.com/`. If omitted, the API uses its `GSC_SITE_URL`, then falls back to a domain property derived from the target URL.

Add DataForSEO rankings, keyword ideas, and organic competitors:

```sh
seoaudit opportunities https://example.com \
  --dataforseo \
  --location "United Kingdom" \
  --language en
```

An uncached run makes four paid live API requests. Results are capped at 25 rows per dataset by default. The report includes the exact provider cost incurred by the current command:

```sh
seoaudit opportunities https://example.com --dataforseo --data-limit 50
```

Use both sources in one opportunity report:

```sh
seoaudit opportunities https://example.com \
  --gsc \
  --dataforseo
```

## Analyse backlinks

Run the separate backlink operation:

```sh
seoaudit backlinks https://example.com --dataforseo
```

An uncached run makes three paid calls for the backlink summary, highest-ranked referring domains, and highest-ranked live backlinks. Search datasets are not requested or charged.

Set the DataForSEO API login and API password in the API server environment before starting it:

```sh
export DATAFORSEO_USERNAME="your-api-login"
export DATAFORSEO_PASSWORD="your-api-password"
```

DataForSEO uses HTTP Basic Authentication rather than a single bearer API key. Obtain these credentials from the DataForSEO API Access dashboard. The API password is separate from the account password. Credentials are never written into reports.

## Track keyword rankings

Create an API-backed tracker by adding selected keywords:

```sh
seoaudit rankings add https://example.com \
  "seo audit tool" \
  "technical seo software" \
  --device both \
  --depth 100
```

Trackers are separated by target, location, and language. Each tracker supports up to 100 keywords. Device can be `desktop`, `mobile`, or `both`; selecting both creates two paid checks per keyword. SERP depth must be a multiple of 10 from 10 to 100.

Run an explicit paid check:

```sh
seoaudit rankings check https://example.com --dataforseo
```

The command uses DataForSEO Google organic live results and records one observation per keyword and device. A keyword not found within the configured depth is stored as an observed not-ranking result rather than an error. Partial provider runs retain successful observations and their exact cost.

Read the latest positions and compare them with the previous stored run without making provider calls:

```sh
seoaudit rankings report https://example.com
```

Remove a keyword while preserving its historical observations:

```sh
seoaudit rankings remove https://example.com "technical seo software"
```

The API retains the latest 100 runs per tracker. Rank checks are manual and scriptable in this version; there is no internal scheduler yet.

## Local API and dashboard

Start the Go REST API and embedded React dashboard:

```sh
seoaudit-api
```

Open `http://127.0.0.1:8787`. The dashboard reads the same SQLite database used by paid-provider snapshots and rank tracking. It shows:

- saved audit health, performance, findings, and page evidence;
- saved Search Console totals and striking-distance queries;
- DataForSEO search visibility and ranked keywords;
- backlink totals and referring domains;
- tracked keyword positions and changes.

The server accepts only a localhost or loopback listen address. Its collection routes require explicit JSON requests, and the dashboard UI remains read-only. It does not send provider credentials or Google tokens to the browser. Use `--db` or `SEOAUDIT_DB_PATH` when the evidence is stored outside the default user configuration directory:

```sh
seoaudit-api --db "$PWD/.seoaudit/provider.db"
```

The dashboard is built with Bun, Vite, and React, then embedded in the `seoaudit-api` binary. It is not linked into the `seoaudit` CLI binary. Bun is not required to run either finished executable.

## SQLite cache and history

The API uses a local SQLite database to avoid repeating identical paid DataForSEO requests and to retain data for later comparisons. Rank tracking uses the same database for normalized configurations, keywords, runs, and keyword/device observations. Public audits and GSC reports only write report snapshots when `--save` is selected.

Complete DataForSEO reports are reused for six hours by default. The cache key includes the provider, operation, normalized target, location, language, and row limit. Partial reports with failed datasets are saved as snapshots but are not cached.

```sh
seoaudit opportunities https://example.com --dataforseo --cache-ttl 12h
seoaudit backlinks https://example.com --dataforseo --refresh
```

`--refresh` bypasses a valid cache entry and performs fresh paid calls. A cache hit reports zero live calls and zero current provider cost while retaining the original retrieval time and original provider cost as separate evidence.

The default database is `seoaudit/seoaudit.db` inside the operating system's user configuration directory. Override it when starting the API with `--db`, or globally with `SEOAUDIT_DB_PATH`:

```sh
export SEOAUDIT_DB_PATH="$PWD/.seoaudit/provider.db"
seoaudit-api
```

Every DataForSEO opportunity or backlink invocation stores a JSON snapshot, including cache hits and partial results. Retention is bounded to the latest 100 snapshots for each provider, operation, and target. Explicitly saved audit and GSC reports use the same retention bound. Rank tracking separately retains the latest 100 normalized runs per tracker. Google Search Console remains live and uncached.

## Search Console authentication

The API process loads authentication in this order:

1. `GSC_ACCESS_TOKEN`, for a short-lived access token;
2. `GOOGLE_APPLICATION_CREDENTIALS`;
3. `~/.config/google-cli/credentials.json`, normally created by `gsetup`;
4. `~/.config/gcloud/application_default_credentials.json`.

OAuth credentials must include the `webmasters.readonly` scope. The integration never writes to Search Console and never includes credentials in reports.

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
- Representative mobile lab performance: FCP, LCP, CLS, TBT, TTFB, DOM/load timing, requests, transfer size, JavaScript/CSS/image weight, third-party requests, DOM size, image dimensions, and offscreen lazy loading
- Content review signals: editorial article author and date evidence, long-form sourcing, subheading use, and paragraph readability
- `opportunities --dataforseo`: ranking distribution, ranked terms, domain-relevant keyword ideas, demand metrics, intent, and organic competitors
- `opportunities --gsc`: clicks, impressions, CTR, average position, striking-distance queries, and observed query overlap
- `backlinks --dataforseo`: provider rank and spam metrics, backlink totals, referring domains, and top live backlink URLs
- `rankings`: persisted target/location/language trackers, desktop and mobile positions, ranking URLs, SERP features, exact provider costs, and previous-run change classification
- local SQLite cache and bounded snapshots for explicitly saved audit, GSC, and DataForSEO operations
- local Go REST execution API, bounded asynchronous jobs, thin CLI proxy, and embedded React dashboard

See [`docs/roadmap.md`](docs/roadmap.md) for planned Lighthouse, content, trust, and authority coverage.

## Boundaries

The audit reports public, observable evidence and labels heuristics as review items. It does not claim access to Google index state, rankings, traffic, keyword demand, backlinks, conversions, Core Web Vitals field data, or AI citations.

DataForSEO results are external provider observations and estimates. Ranking snapshots, search volume, estimated traffic, backlink coverage, rank, and spam metrics are not Google first-party measurements or ranking guarantees. Google Search Console remains the source for a site's actual Google clicks and impressions.

Search Console results are authenticated Google first-party measurements for the selected property and period. The API returns top rows subject to its aggregation and data limits, so totals in the report describe the returned dataset rather than guaranteed complete property totals. Positions 4–20 and multi-page query observations are prioritisation views, not ranking guarantees or proof of keyword cannibalisation.

The crawler analyses server-returned HTML and automatically uses local Chrome for pages that expose very little raw content. Performance results are reproducible lab diagnostics under a simulated mobile profile, not Chrome User Experience Report field data or a Google ranking guarantee. INP requires real interaction data and is therefore not invented by the lab audit; TBT is reported as its development-time proxy.

Deliberate `noindex` directives remain visible in page-level indexability data. They become actionable failures when a non-indexable URL is also submitted in a sitemap.

## Development

```sh
go test ./...
go vet ./...
go build ./cmd/seoaudit ./cmd/seoaudit-api
cd dashboard
bun install
bun run build
```
