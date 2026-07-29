# Audit Coverage

The public audit follows this priority order:

1. crawlability and indexability;
2. response and canonical integrity;
3. page titles, descriptions, headings, and visible content;
4. internal links, crawl depth, duplication, and sitemap consistency;
5. content clarity, article trust signals, and long-form readability;
6. images, resources, schema syntax, hreflang, mobile basics, and public AI crawler access.

## Evidence policy

- Every finding must name the affected URL and observable evidence.
- Missing optional markup is not automatically an SEO failure.
- Character lengths, word counts, depth, image size, and similarity are heuristics for review rather than ranking rules.
- Structured data syntax does not prove Google rich-result eligibility.
- A sitemap URL is expected to be successful, canonical, and indexable.
- A URL found only in a sitemap is an orphan candidate, not proof that no link exists anywhere.
- Deliberate `noindex` directives remain visible in each page's indexability evidence but are not standalone action items. A noindexed sitemap URL is still a failure.
- Page URL-format heuristics apply to HTML documents, not public assets whose filenames follow an external convention.
- Verbose mode streams API job events for robots, sitemap, page crawl, analysis, and linked-resource progress to stderr. It does not change audit coverage or JSON stdout.
- Local Chrome tests the homepage plus one page from each of the largest indexable path sections, capped at six pages per audit.
- Performance uses a simulated mobile viewport, CPU slowdown, network throttling, disabled cache, and an isolated tab per measured page.
- Lab performance includes FCP, LCP, CLS, observed TBT, TTFB, page milestones, transfer/request weight, DOM size, and rendered-image diagnostics.
- INP is field-only in this context. The audit reports TBT as a lab proxy and does not claim Core Web Vitals field compliance.
- Content review checks cover author and date signals on editorial articles, external sourcing on long articles, subheading use, and unusually long paragraphs.
- Content review findings are low-priority prompts for human review. They are not claims that a page is unhelpful or unable to rank.
- `audit --save` asks the API to store the completed JSON report for the local dashboard. Without that explicit flag, the public audit does not read or write audit snapshots.

## Public-only limitations

- Google-selected canonicals and real index status require Search Console.
- Core Web Vitals field data requires CrUX or PageSpeed data.
- Search demand, rankings, backlinks, and conversions cannot be inferred from the website alone.
- Sparse client-rendered pages are automatically rendered with local Chrome when available.
- JavaScript changes on otherwise content-rich pages can still require separate raw-versus-rendered validation.
- Search intent, originality, factual accuracy, expertise, and comparative content quality require human or explicitly enabled model review.

## Search opportunity data

`seoaudit opportunities <url> --dataforseo` adds four direct DataForSEO REST datasets:

- organic ranking distribution and estimated visibility;
- ranked keywords and ranking URLs;
- domain-relevant keyword ideas with demand and competition metrics when available;
- organic competitors based on keyword overlap;

The report records source, target, location, language, retrieval time, successful datasets, live-call count, dataset errors, cache evidence, snapshot ID, and the exact provider cost incurred by the current command. External estimates are not presented as Google Search Console or analytics measurements.

`seoaudit opportunities <url> --gsc` adds one authenticated, read-only Google Search Analytics request:

- finalized clicks, impressions, CTR, and average position grouped by query and page;
- returned-dataset totals and impression-weighted position;
- query/page rows in observed positions 4–20, sorted by impressions;
- queries observed against multiple pages in the returned dataset.

The request is bounded by a configurable 1–25,000 row limit and a maximum 480-day lookback. The default is 250 rows over 28 days. Search Console can omit anonymized queries and rows beyond its API limits, so returned totals are not represented as complete property totals. Multi-page query observations are candidates for review rather than proof that pages compete with each other. `--save` explicitly retains the returned report for the local dashboard.

## Backlink data

`seoaudit backlinks <url> --dataforseo` adds three direct DataForSEO REST datasets:

- backlink summary and provider authority signals;
- highest-ranked referring domains;
- highest-ranked individual live backlinks.

The backlink operation records provider cost and errors independently. It does not request or charge for keyword, ranking, or competitor datasets.

## Paid-provider persistence

The API owns its local SQLite database. Rank tracker configuration, reports, provider cache entries, and snapshots never pass through the CLI. The public audit and unsaved GSC-only opportunity execution paths do not read or write their report snapshot tables.

- Complete DataForSEO reports are cached for six hours by default.
- The cache key includes provider, dataset group, normalized target, location, language, and row limit.
- `--cache-ttl` changes the reuse window and `--refresh` forces new paid calls.
- Cache hits retain the source retrieval time and original provider cost, but report zero live calls and zero current-command provider cost.
- Every DataForSEO opportunity or backlink invocation writes a JSON snapshot, including cache hits and partial reports.
- Partial reports are not cached, so a transient dataset failure does not become the reusable result.
- Snapshot retention is bounded to the latest 100 records for each provider, dataset group, and target.
- `seoaudit-api --db` or `SEOAUDIT_DB_PATH` overrides the operating-system user configuration path.

## Rank tracking

`seoaudit rankings` provides a separate persisted workflow:

- `add` creates or updates a target/location/language tracker and adds normalized, deduplicated keywords;
- `remove` stops tracking selected keywords without deleting historical observations;
- `check --dataforseo` explicitly runs paid Google organic live checks;
- `report` reads current and previous stored results without paid calls.

Checks are bounded to 100 keywords, desktop and/or mobile, and an organic depth from 10 to 100. DataForSEO requests stop crawling once the target domain or a subdomain is found and restrict target matching to organic results. Each successful keyword/device task stores position, ranking URL, and discovered SERP element types. A null position means the task completed but the target was not found within the configured depth.

Reports classify comparable observations as improved, declined, new, lost, or stable. A newly tracked term without a prior observation is uncompared; a missing result from a partial provider run is not checked and is not described as not ranking. Exact current-run provider cost, task counts, partial errors, and retrieval time remain attached to each run. Retention is bounded to 100 runs per tracker.

Rank checks currently use live DataForSEO requests and run only when explicitly invoked. Automatic scheduling and the lower-cost standard task queue are not implemented.

## Local API and dashboard

`seoaudit-api` starts the localhost-only Go REST API with the embedded Bun, Vite, and React frontend. The separate `seoaudit` executable contains only proxy commands and terminal formatting.

- `GET /api/v1/health` reports server availability.
- `GET /api/v1/capabilities` reports bounded API limits and whether provider credentials are configured without returning secrets.
- `POST /api/v1/audits`, `/opportunities`, and `/backlinks` create bounded asynchronous jobs.
- `GET /api/v1/jobs/{id}`, `/events`, and `/result` expose lifecycle, progress, and complete results; `DELETE /api/v1/jobs/{id}` requests cancellation.
- `GET`, `POST`, and `PATCH` routes under `/api/v1/rank-trackers` own tracker reads, mutations, and explicit paid checks.
- `GET /api/v1/sites` lists targets found across saved reports, provider snapshots, and rank trackers.
- `GET /api/v1/sites/{target}` joins the latest saved audit, GSC, DataForSEO search, backlink, and ranking evidence.
- API responses never contain provider or Google credentials.
- The frontend receives dashboard summaries and bounded evidence rows rather than direct SQLite access.

The dashboard UI does not trigger crawls or paid calls. The explicit REST operations own data collection, and the CLI is a light proxy to them. See [`docs/api.md`](api.md) for the complete contract.
