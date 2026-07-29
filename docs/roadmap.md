# Roadmap

The API separates public auditing from account-backed and paid datasets, while the CLI proxies the same explicit operations:

```sh
seoaudit audit https://example.com
seoaudit opportunities https://example.com --gsc
seoaudit opportunities https://example.com --dataforseo
seoaudit backlinks https://example.com --dataforseo
seoaudit rankings check https://example.com --dataforseo
seoaudit-api
```

New coverage should remain explainable, bounded, and useful to someone who does not work in SEO. Account-backed and paid sources remain explicit and retain their source semantics.

## Current: content and trust foundations

- Detect long pages with no descriptive subheadings.
- Flag paragraphs that are unusually difficult to scan.
- Detect public author and published or updated date signals on editorial articles.
- Review long articles that make no links to external sources.
- Keep these as low-priority review prompts rather than hard SEO failures.

## Planned: full Lighthouse diagnostics

- Run real Lighthouse on the homepage and representative templates.
- Report performance, accessibility, best-practice, and SEO category scores.
- Surface actionable opportunities such as render-blocking resources, inefficient cache lifetimes, legacy JavaScript, unused code, image delivery, and font loading.
- Preserve dependency trees, LCP breakdowns, layout-shift culprits, and third-party impact as diagnostics rather than failures.
- Keep the existing fast Chrome measurements for crawl-wide regression checks.

## Planned: site-wide content system

- Compare titles, main headings, introductions, and anchor text with the page's stated topic.
- Group pages by likely topic and identify pages that may compete for the same intent.
- Identify weak hub-to-detail relationships and content clusters with poor internal support.
- Detect excessive template boilerplate and pages with too little unique main content.
- Find stale dates and time-sensitive claims that need editorial review.
- Review About, Contact, editorial policy, author profile, privacy, and terms visibility as site-wide trust signals.

## Planned: GEO and answer readiness

- Detect concise definitions, direct answers, comparison tables, FAQs, steps, and source citations.
- Validate `llms.txt` structure and linked URLs.
- Check whether AI search crawlers are publicly allowed.
- Report machine-readable pricing, API documentation, and agent instructions when present.
- Treat these as discoverability aids, not guarantees of AI citations.

## Optional integrations

- PageSpeed Insights and CrUX for real-user Core Web Vitals when public field data exists.
- Expand the current Search Console query/page integration with bounded URL Inspection and period-over-period content decay.
- An optional LLM review for intent satisfaction, originality, clarity, and expertise.
- DataForSEO rankings, keyword research, and competitors are available under `opportunities --dataforseo`.
- DataForSEO backlinks, referring domains, and provider authority metrics are available under `backlinks --dataforseo`.
- Manual DataForSEO rank tracking with normalized SQLite history is available under `rankings`; queued scheduled checks remain planned.

The public audit will continue to work without accounts or paid SEO providers and will not persist a report unless explicitly requested.

DataForSEO operations use a bounded local SQLite cache and snapshot history owned by the API. Rank tracking adds normalized keyword/device history for direct comparisons. The local dashboard reads these records plus explicitly saved audit and Search Console snapshots without changing the default public audit contract.
