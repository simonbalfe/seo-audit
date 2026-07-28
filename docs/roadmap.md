# Roadmap

Each stage remains small and usable before the next one begins.

| Stage | Status | Commands | Scope |
|---|---|---|---|
| 1 | Built | `audit`, `page`, `robots`, `sitemap` | One public page, crawler access, and sitemap discovery |
| 2 | Planned | `crawl`, `links` | Bounded same-domain crawl, redirects, broken links, and duplicate metadata |
| 3 | Planned | `structure`, `content` | Crawl depth, orphan candidates, internal-link graph, content inventory, and repeated content |
| 4 | Planned | `geo` | Public AI access, extractable facts, entity consistency, evidence, authorship, and freshness |
| 5 | Planned | `snapshot`, `compare`, `report` | Local saved baselines, change detection, prioritised actions, and export |

## Data policy

Every stage reads the target website directly. No Search Console, Analytics, rank tracker, backlink provider, keyword-volume provider, or AI visibility platform is required.

If external data is added later, it must be a separate optional adapter and must not change the meaning of public-site findings.
