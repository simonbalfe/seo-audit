# Audit Coverage

The public audit follows this priority order:

1. crawlability and indexability;
2. response and canonical integrity;
3. page titles, descriptions, headings, and visible content;
4. internal links, crawl depth, duplication, and sitemap consistency;
5. images, resources, schema syntax, hreflang, mobile basics, and public AI crawler access.

## Evidence policy

- Every finding must name the affected URL and observable evidence.
- Missing optional markup is not automatically an SEO failure.
- Character lengths, word counts, depth, image size, and similarity are heuristics for review rather than ranking rules.
- Structured data syntax does not prove Google rich-result eligibility.
- A sitemap URL is expected to be successful, canonical, and indexable.
- A URL found only in a sitemap is an orphan candidate, not proof that no link exists anywhere.
- Deliberate `noindex` directives remain visible in each page's indexability evidence but are not standalone action items. A noindexed sitemap URL is still a failure.
- Page URL-format heuristics apply to HTML documents, not public assets whose filenames follow an external convention.
- Verbose mode reports robots, sitemap, page crawl, analysis, and linked-resource progress to stderr. It does not change audit coverage or JSON stdout.

## Public-only limitations

- Google-selected canonicals and real index status require Search Console.
- Core Web Vitals field data requires CrUX or PageSpeed data.
- Search demand, rankings, backlinks, and conversions cannot be inferred from the website alone.
- Sparse client-rendered pages are automatically rendered with local Chrome when available.
- JavaScript changes on otherwise content-rich pages can still require separate raw-versus-rendered validation.
