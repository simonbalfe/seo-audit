# Local SEO Audit Checklist

Status:

- **Yes**: implemented and included in the audit.
- **Optional**: implemented only when its flag is used.
- **No**: not implemented.

## Business and Google profile

| Check | Status |
|---|---|
| Resolve the exact Google Place ID and public website, with an explicit `--website` override for an unlinked GBP | Yes |
| Record business name, category, address, phone, website, status, hours, rating, reviews, photos, and Maps URL | Yes |
| Verify that the profile and website represent the same business | Yes |
| Flag missing public profile fields | Yes |
| Compare primary and secondary categories with competitors | No |
| Check description, services, products, posts, questions, links, and attributes | No |
| Check special hours, service areas, and storefront settings | No |
| Compare photo and video coverage with competitors | No |
| Detect duplicate or conflicting profiles | No |

## Keywords, rankings, and competitors

| Check | Status |
|---|---|
| Discover commercial keyword ideas from the website and GBP category | Yes |
| Generate bounded commercial keyword seeds from crawled page metadata | Yes |
| Pull and save existing UK organic rankings before new keyword research | Yes |
| Accept supplied keywords and retrieve volume and CPC | Yes |
| Remove branded, informational, irrelevant, and duplicate ideas | Yes |
| Recheck priority existing keywords at the GBP location in organic search | Yes |
| Recheck the same keywords in Maps by exact Place ID | Yes |
| Keep current visibility separate from new keyword opportunities | Yes |
| Collect nearby Maps competitors, categories, ratings, reviews, URLs, and coordinates | Yes |
| Select up to five non-brand, validated keywords across distinct commercial pages for the Maps grid | Yes |
| Measure a 3x3 Maps rank grid and report checked points, failed points, median rank, and coverage | Yes |
| Create prioritised opportunities from weak organic and Maps visibility | Yes |
| Collect organic competitors from the same search results | No |
| Compare the business directly with profiles ranking above it | No |
| Support custom grid density and service-area shapes | No |

## Website relevance and conversion

| Check | Status |
|---|---|
| Classify every crawled page by purpose during visibility research | Yes |
| Cache unchanged AI page research | Yes |
| Identify priority commercial pages and map validated keywords to them | Yes |
| Detect phone and booking actions on ranking pages | Yes |
| Map services and target locations to dedicated pages | No |
| Check local intent in titles, headings, copy, and internal links | No |
| Check local proof, directions, parking, accessibility, and service-area content | No |
| Compare name, address, phone, hours, categories, and services across GBP, website, and schema | No |
| Confirm the GBP website URL is live, canonical, indexable, and points to the right location page | No |
| Validate rendered LocalBusiness schema against GBP and visible content | No |

## Reputation and authority

| Check | Status |
|---|---|
| Record public rating and review count | Yes |
| Compare ratings and review counts with ranking competitors | No |
| Analyse review recency, velocity, replies, and themes | No |
| Check major citations and name, address, and phone consistency | No |
| Summarise backlinks, referring domains, nofollow domains, spam score, broken backlinks, and leading countries | Yes |
| Inspect individual backlinks, local relevance, competitor link gaps, and brand mentions | No |

## Technical and on-page SEO

| Check | Status |
|---|---|
| HTTP status, redirects, robots.txt, sitemaps, indexability, canonicals, and hreflang | Yes |
| Broken and redirected internal links, internal nofollow, empty anchors, depth, and sitemap-only pages | Yes |
| External links and linked resources | Yes |
| Titles, descriptions, H1s, heading order, thin content, and duplicate content | Yes |
| Image alt-text coverage and large images | Yes |
| JSON-LD syntax and discovered schema types | Yes |
| Mobile lab FCP, LCP, CLS, TBT, TTFB, request count, transfer size, JavaScript, and DOM size | Yes |
| Standard PageSpeed Insights mobile audit for the homepage | No |
| Field Core Web Vitals including INP | No |

### Performance improvement

- Replace the six sequential custom Chrome performance checks with one mobile PageSpeed Insights homepage check.
- Use the returned Lighthouse metrics and diagnostics instead of maintaining custom metric calculations.
- Query the CrUX API separately when field data is available. Missing CrUX data is not a failure.
- Keep Chrome only for rendering JavaScript-dependent pages during the website crawl.

## Reporting

| Check | Status |
|---|---|
| Prioritised terminal report | Yes |
| Complete timestamped JSON evidence | Yes |
| Keyword, organic, Maps, grid, competitor, and opportunity summary | Yes |
| Provider calls, cost, crawl coverage, and limit status | Yes |
| Saved page summaries grouped by Place ID | Yes |
| Local dashboard with full and per-workflow audit runs, parameters, live progress, and logs | Yes |
| Mark every module as completed, skipped, or unavailable | No |
| Group repeated issues by unique problem and affected pages | No |
| Compare audits over time | No |

## Build next

1. Replace custom Chrome performance checks with one PageSpeed Insights mobile check.
2. Compare the target profile with competitors ranking above it.
3. Compare GBP details with website content and LocalBusiness schema.
4. Add configurable Maps grid density and service-area shapes.
5. Add review, citation, and competitor backlink-gap checks.

## Boundaries

- Public data only. No Search Console, Analytics, CRM, or private GBP access.
- Rankings and findings use measured evidence.
- AI routes pages and proposes natural commercial keyword seeds. DataForSEO validates demand and rankings. AI never creates rankings or pass/fail findings.
- The report identifies opportunities, not ranking guarantees.
- Crawl and provider limits are listed in the [`README`](../README.md#crawl-limits).
