# SEO Audit

Audit a local business from its exact Google Place ID.

```sh
seoaudit audit ChIJQ-rbABcbcUgRW9BTCMyiHAI
```

The Place ID supplies the public website, business profile, coordinates, category, and country. The audit then checks:

- website crawlability, indexability, redirects, canonicals, and sitemaps;
- titles, descriptions, headings, content, links, images, and schema;
- mobile performance on representative pages;
- public Google Business Profile details;
- relevant local keywords and existing rankings;
- priority commercial pages matched to validated keywords;
- live organic and exact Place-ID Maps positions;
- nearby Maps competitors;
- a 3x3 Maps rank grid over 2 km;
- backlink totals, referring domains, spam score, broken backlinks, and leading countries;
- prioritised local SEO opportunities.

Visibility has two separate stages. The current snapshot pulls existing UK rankings and rechecks up to five at the GBP location in Search and Maps. Opportunity research then finds and checks up to five new keywords. One 3x3 grid runs for up to five proven, non-brand commercial terms: existing rankings first, then distinct service-page opportunities. Brand terms, postcodes, duplicates, and terms with no measured demand are excluded.

The full implementation checklist is in [`docs/coverage.md`](docs/coverage.md).

## Options

```sh
seoaudit audit <place-id> [options]
```

| Option | Purpose |
|---|---|
| `--keyword "query"` | Add a target query. Repeat as needed. |
| `--steps <workflow>` | Run `all`, `website`, `performance`, `visibility`, `backlinks`, or `profile`. |
| `--website https://example.com` | Use a verified canonical website when the GBP has none. |
| `--limit 50` | Set the page limit from 1 to 5000. |
| `--timeout 30s` | Set each request timeout up to 120 seconds. |
| `--external=false` | Skip external-link checks. |
| `--performance=false` | Skip Chrome performance checks. |
| `--json` | Print the complete JSON report. |
| `--output audit.json` | Choose the JSON output path. |
| `--debug` | Show audit progress and fetched pages. |

Open the local dashboard:

```sh
seoaudit audit --dashboard
```

The dashboard can run a full audit or rerun one workflow. It shows live progress and logs. You can change the crawl limit, request timeout, external-link checks, full-audit performance checks, and supplied keywords before starting.

Run a workflow from the terminal:

```sh
seoaudit audit <place-id> --steps backlinks
```

| Workflow | Runs |
|---|---|
| `all` | GBP, website, performance, visibility, and backlinks |
| `website` | GBP plus crawl, technical, on-page, and link checks |
| `performance` | GBP, crawl, and representative mobile lab tests |
| `visibility` | GBP, crawl, page research, keywords, Search, Maps, competitors, and grid |
| `backlinks` | GBP plus one domain backlink summary |
| `profile` | Public GBP details only |

## Crawl limits

The crawler follows same-host links breadth-first from the homepage, then uses remaining capacity for unseen sitemap URLs.

| Limit | Value |
|---|---:|
| Pages | 50 default, 5000 maximum |
| Request timeout | 30 seconds default, 120 maximum |
| Page fetches in flight | 16 |
| Chrome renders in flight | 4 |
| Resource checks in flight | 24 |
| Page size | 5 MiB |
| Redirects | 10 per page |
| Sitemap size | 10 MiB |
| Performance pages | 6 |
| Current visibility queries | 5 existing rankings |
| Opportunity queries | 5 new keywords |
| Maps grid | 3x3 over 2 km for up to 5 selected keywords |
| Backlink summary | 1 live call |
| DataForSEO requests in flight | 20 |

Pages with fewer than 50 words in raw HTML are rendered in Chrome. The report records when the page limit is reached.

## Credentials

Always required:

- `GOOGLE_MAPS_API_KEY`

Required for `all`, `visibility`, and `backlinks`:

- `DATAFORSEO_USERNAME`
- `DATAFORSEO_PASSWORD`

Required for `all` and `visibility`:

- `OPENROUTER_API_KEY`

Credentials load from the process environment, the current directory's `.env`, then the platform config file at `seoaudit/.env`. Existing environment values win. Google Places and DataForSEO calls are live and paid.

## Output

Each audit:

- prints a short terminal report;
- saves complete JSON evidence under `output/` unless `--output` is used;
- updates `output/audits.sqlite` for the local dashboard;
- stores cached visibility page research in `classifications.sqlite`.

## Private API

The production container runs `seoaudit serve` on the Google Maps VPS,
Tailscale-only at port 8090, deployed by the leads app's production workflow. It accepts one request at a time:

It resolves the supplied Place ID once, then runs only the DataForSEO visibility scan. It does not crawl the website or call OpenRouter.

```http
POST /api/audits
Content-Type: application/json

{"placeId":"ChIJQ-rbABcbcUgRW9BTCMyiHAI"}
```

It runs the evidence-led visibility audit and returns the full JSON report.
Send `Accept: application/pdf` to download the same DataForSEO visibility results as a client-facing PDF.
