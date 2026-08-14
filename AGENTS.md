# SEO Audit guidance

SEO Audit has one purpose: use public evidence to show how a local business currently appears in Google, where it is visible, who outranks it, and which gaps create practical SEO opportunities. Everything runs through one `seoaudit audit <place-id>` command. The exact Google Place ID resolves the public website and anchors every check.

## Product contract

- Keep exactly one user-facing audit command. Do not add separate commands for GBP, Maps, keywords, competitors, performance, backlinks, citations, or reporting.
- Keep flags limited to output, diagnostics, crawl bounds, supplied keywords, the dashboard, and `--steps all|website|performance|visibility|backlinks|profile`. Do not expose provider location, language, business search, market-check-count, grid-radius, or AI flags.
- Make `seoaudit audit --help` state the complete default audit, crawl bounds, paid calls, and required credentials.
- Keep the core audit public-only. Do not require access to the business's Search Console, Analytics, CRM, or private GBP account.
- Always resolve the exact Place ID and run public technical, on-page, performance, external-link, Google Business Profile, DataForSEO backlink summary, keyword, organic rank, exact Maps rank, competitor, and bounded geo-grid checks.
- Keep performance and external-link checks individually optional.
- Partial workflows resolve the Place ID, run only the selected audit area and its required crawl, then save JSON. `--steps backlinks` skips crawling and makes one DataForSEO backlink-summary call. `--steps profile` does not require a public website.
- Treat OpenRouter page research as an internal part of local visibility, never a separate step or flag. Merge any supplied `--keyword` values into the default local visibility shortlist.
- Keep current visibility and new opportunities separate. First pull and report DataForSEO existing organic rankings and recheck up to five with live local organic and exact Maps. Then use priority pages, OpenRouter seeds, site ideas, and supplied keywords to check up to five new opportunities.
- Run one bounded 3x3 geo-grid for up to five validated non-brand commercial terms. Prefer existing ranking terms, then fill from distinct commercial-page opportunities. Exclude brand terms, postcodes, duplicate pages, and zero-demand terms. Record failed grid points separately and exclude them from rank coverage.
- Use resolved GBP coordinates for local DataForSEO keyword and organic requests and the GBP country for country-level ranked-keyword discovery; never pass an unverified short market label as a provider location name when exact GBP data is available.
- Keep DataForSEO and Google Places requests live. Cache only OpenRouter page classifications in SQLite, keyed by URL, model, and the exact public metadata fingerprint sent for classification.
- Send every crawled page's URL, title, H1, schema types, and action presence to OpenRouter. Never send page bodies or credentials. Use the response only to identify priority commercial pages and propose bounded keyword seeds.
- Keep AI research separate from deterministic findings. AI may route pages and propose keywords but must not invent evidence, rankings, scores, or pass/fail results.
- Store the classification cache beside the JSON output as `classifications.sqlite`. Reuse unchanged classifications, send changed or missing pages to OpenRouter, and report cache hits separately from provider requests.
- Validate only the credentials required by the selected workflow before resolving the Place ID or starting the crawl. A missing credential must fail fast with the exact required environment-variable name.
- Load provider credentials without overwriting the process environment, then check the current directory's `.env`, then the platform user-config file at `seoaudit/.env`. Never print credential values.
- Keep requests bounded by page size, timeout, redirect count, sitemap count, crawl limit, market-check count, and one backlink-summary call.
- Keep public crawl analysis and Chrome performance measurement under `internal/audit/crawl` and `internal/audit/performance`; compose them through `internal/audit`.
- Keep findings deterministic and tied to fetched evidence.
- Do not describe heuristics as search-engine rules or ranking guarantees.
- Do not report missing optional schema as an error.
- Keep terminal output prioritised and concise while preserving full evidence in JSON.
- Persist accumulated crawled-page summaries in `output/audits.sqlite`, keyed by resolved Google Place ID. Do not turn this into audit-history or job storage without an explicit request.
- Serve the embedded Vite and React dashboard through `seoaudit audit --dashboard` on `127.0.0.1:4173`. It lists saved businesses and pages and runs one validated local audit job at a time with live progress.

## Change discipline

- Update `docs/coverage.md` when audit behaviour changes.
- Update `README.md` when flags, coverage, or observable output changes.
- Run `go test ./...`, `go vet ./...`, and `go build ./cmd/seoaudit` after substantive changes.
- Run `npm run build` in `dashboard/` after frontend changes so the embedded assets match the source.
- After every repository change, run `go install ./cmd/seoaudit` so the `seoaudit` command on `PATH` matches the working tree.
