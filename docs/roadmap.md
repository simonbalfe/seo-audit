# Roadmap

The CLI keeps one user-facing operation:

```sh
seoaudit audit https://example.com
```

New coverage should remain public-site-only, explainable, bounded, and useful to someone who does not work in SEO.

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
- Search Console for queries, impressions, indexing evidence, and content decay.
- An optional LLM review for intent satisfaction, originality, clarity, and expertise.
- DataForSEO rankings, keyword research, competitors, backlinks, referring domains, and provider authority metrics are available through the explicit `--dataforseo` flag.

The default audit will continue to work without accounts, a database, or paid SEO providers.
