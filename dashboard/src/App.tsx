import { useEffect, useState } from "react";
import {
  loadSite,
  loadSites,
  type Ranking,
  type SiteResponse,
  type SiteSummary,
} from "./api";

type DashboardState =
  | { status: "loading" }
  | { status: "empty" }
  | { status: "error"; message: string }
  | {
      status: "ready";
      sites: readonly SiteSummary[];
      selectedTarget: string;
      site: SiteResponse;
      refreshing: boolean;
    };

const sections = [
  ["overview", "Overview"],
  ["site-health", "Site health"],
  ["search", "Search"],
  ["backlinks", "Backlinks"],
  ["rankings", "Rankings"],
] as const;

export function App(): React.ReactNode {
  const [state, setState] = useState<DashboardState>({ status: "loading" });

  useEffect(() => {
    const controller = new AbortController();
    void loadInitialData(controller.signal, setState);
    return () => {
      controller.abort();
    };
  }, []);

  const selectSite = async (target: string): Promise<void> => {
    if (state.status !== "ready" || target === state.selectedTarget) {
      return;
    }
    setState({ ...state, selectedTarget: target, refreshing: true });
    try {
      const site = await loadSite(target);
      setState({
        status: "ready",
        sites: state.sites,
        selectedTarget: target,
        site,
        refreshing: false,
      });
    } catch (error: unknown) {
      setState({
        status: "error",
        message: error instanceof Error ? error.message : "Could not load the selected site",
      });
    }
  };

  if (state.status === "loading") {
    return <LoadingState />;
  }
  if (state.status === "empty") {
    return <EmptyState />;
  }
  if (state.status === "error") {
    return <ErrorState message={state.message} />;
  }

  return (
    <div className="app-shell">
      <Sidebar target={state.site.target} />
      <main className="main">
        <Topbar
          sites={state.sites}
          selectedTarget={state.selectedTarget}
          refreshing={state.refreshing}
          onSelect={selectSite}
        />
        <DashboardContent site={state.site} />
      </main>
    </div>
  );
}

async function loadInitialData(
  signal: AbortSignal,
  setState: React.Dispatch<React.SetStateAction<DashboardState>>,
): Promise<void> {
  try {
    const response = await loadSites(signal);
    const firstSite = response.sites[0];
    if (firstSite === undefined) {
      setState({ status: "empty" });
      return;
    }
    const site = await loadSite(firstSite.target);
    setState({
      status: "ready",
      sites: response.sites,
      selectedTarget: firstSite.target,
      site,
      refreshing: false,
    });
  } catch (error: unknown) {
    if (error instanceof DOMException && error.name === "AbortError") {
      return;
    }
    setState({
      status: "error",
      message: error instanceof Error ? error.message : "Could not load dashboard data",
    });
  }
}

function Sidebar({ target }: { readonly target: string }): React.ReactNode {
  return (
    <aside className="sidebar">
      <div className="brand">
        <span className="brand-mark">S</span>
        <span>SEO Audit</span>
      </div>
      <div className="site-pill">
        <span className="site-dot" />
        <span>{target}</span>
      </div>
      <nav aria-label="Dashboard sections">
        {sections.map(([id, label], index) => (
          <a className={index === 0 ? "nav-link active" : "nav-link"} href={`#${id}`} key={id}>
            <span className="nav-index">0{index + 1}</span>
            {label}
          </a>
        ))}
      </nav>
      <div className="sidebar-foot">
        <span className="source-light" />
        Local evidence
      </div>
    </aside>
  );
}

type TopbarProps = {
  readonly sites: readonly SiteSummary[];
  readonly selectedTarget: string;
  readonly refreshing: boolean;
  readonly onSelect: (target: string) => Promise<void>;
};

function Topbar({ sites, selectedTarget, refreshing, onSelect }: TopbarProps): React.ReactNode {
  return (
    <header className="topbar">
      <div>
        <p className="eyebrow">Workspace</p>
        <h1>{selectedTarget}</h1>
      </div>
      <div className="topbar-actions">
        {refreshing ? <span className="sync-label">Loading…</span> : <span className="sync-label">Synced</span>}
        <label className="site-select">
          <span className="sr-only">Choose site</span>
          <select
            value={selectedTarget}
            onChange={(event) => {
              void onSelect(event.target.value);
            }}
          >
            {sites.map((site) => (
              <option key={site.target} value={site.target}>
                {site.target}
              </option>
            ))}
          </select>
        </label>
      </div>
    </header>
  );
}

function DashboardContent({ site }: { readonly site: SiteResponse }): React.ReactNode {
  const ranking = site.rankings[0];
  const metrics = dashboardMetrics(site, ranking);
  return (
    <div className="content">
      <section className="intro" id="overview">
        <div>
          <p className="eyebrow">Evidence overview</p>
          <h2>Search health, without the noise.</h2>
          <p className="intro-copy">
            Current crawl, search, authority and ranking evidence from the local SEO Audit database.
          </p>
        </div>
        <div className="updated-card">
          <span>Last evidence</span>
          <strong>{formatDate(site.last_updated)}</strong>
        </div>
      </section>

      {site.warnings !== undefined && site.warnings.length > 0 ? (
        <div className="warning-strip">{site.warnings.join(" ")}</div>
      ) : null}

      <section className="metric-grid" aria-label="Key metrics">
        {metrics.map((metric) => (
          <article className="metric-card" key={metric.label}>
            <div className={`metric-icon ${metric.tone}`}>{metric.short}</div>
            <p>{metric.label}</p>
            <strong>{metric.value}</strong>
            <span>{metric.detail}</span>
          </article>
        ))}
      </section>

      <div className="two-column">
        <RankDistribution ranking={ranking} />
        <EvidenceSources site={site} />
      </div>

      <AuditSection site={site} />
      <SearchSection site={site} />
      <BacklinkSection site={site} />
      <RankingSection ranking={ranking} />
    </div>
  );
}

type Metric = {
  readonly label: string;
  readonly value: string;
  readonly detail: string;
  readonly short: string;
  readonly tone: string;
};

function dashboardMetrics(site: SiteResponse, ranking: Ranking | undefined): readonly Metric[] {
  const auditActions =
    site.audit === undefined ? undefined : site.audit.summary.failures + site.audit.summary.warnings;
  const impressions = site.gsc?.summary.returned_impressions;
  const referringDomains = site.backlinks?.backlink_summary.referring_domains;
  return [
    {
      label: "Indexable pages",
      value: formatOptional(site.audit?.summary.indexable),
      detail:
        site.audit === undefined
          ? "Save an audit run"
          : `${site.audit.summary.pages} pages crawled`,
      short: "SH",
      tone: "violet",
    },
    {
      label: "Audit actions",
      value: formatOptional(auditActions),
      detail:
        site.audit === undefined
          ? "No saved crawl"
          : `${site.audit.summary.failures} failures · ${site.audit.summary.warnings} warnings`,
      short: "AU",
      tone: "teal",
    },
    {
      label: "Search impressions",
      value: formatCompact(impressions),
      detail: site.gsc === undefined ? "Save a GSC run" : `${formatCompact(site.gsc.summary.returned_clicks)} clicks`,
      short: "GS",
      tone: "blue",
    },
    {
      label: "Top 100 rankings",
      value: formatOptional(ranking?.summary.ranking),
      detail: ranking === undefined ? "No tracker" : `${ranking.summary.top_10} in the top 10`,
      short: "RK",
      tone: "purple",
    },
    {
      label: "Referring domains",
      value: formatOptional(referringDomains),
      detail: site.backlinks === undefined ? "No saved backlink run" : `${site.backlinks.backlink_summary.backlinks} backlinks`,
      short: "BL",
      tone: "green",
    },
  ];
}

function RankDistribution({ ranking }: { readonly ranking: Ranking | undefined }): React.ReactNode {
  if (ranking === undefined) {
    return (
      <section className="panel chart-panel">
        <PanelHeading eyebrow="Rankings" title="Position distribution" />
        <EmptyPanel text="Add tracked keywords to populate ranking distribution." />
      </section>
    );
  }
  const summary = ranking.summary;
  const distribution = [
    { label: "Top 3", value: summary.top_3, tone: "top3" },
    { label: "4–10", value: Math.max(0, summary.top_10 - summary.top_3), tone: "top10" },
    { label: "11–100", value: Math.max(0, summary.ranking - summary.top_10), tone: "top100" },
    { label: "Outside 100", value: summary.not_ranking, tone: "outside" },
  ];
  const max = Math.max(1, ...distribution.map((item) => item.value));
  return (
    <section className="panel chart-panel">
      <PanelHeading eyebrow="Rankings" title="Position distribution" />
      <div className="bar-chart">
        {distribution.map((item) => (
          <div className="bar-row" key={item.label}>
            <span>{item.label}</span>
            <div className="bar-track">
              <div
                className={`bar-fill ${item.tone}`}
                style={{ width: `${Math.max(4, (item.value / max) * 100)}%` }}
              />
            </div>
            <strong>{item.value}</strong>
          </div>
        ))}
      </div>
      <p className="panel-note">
        {ranking.config.location} · {ranking.config.devices} · depth {ranking.config.serp_depth}
      </p>
    </section>
  );
}

function EvidenceSources({ site }: { readonly site: SiteResponse }): React.ReactNode {
  const sources = [
    ["Public crawl", site.audit !== undefined],
    ["Search Console", site.gsc !== undefined],
    ["Search opportunity data", site.search !== undefined],
    ["Backlink index", site.backlinks !== undefined],
    ["Rank tracking", site.rankings.length > 0],
  ] as const;
  return (
    <section className="panel">
      <PanelHeading eyebrow="Coverage" title="Connected evidence" />
      <div className="source-list">
        {sources.map(([label, available]) => (
          <div className="source-row" key={label}>
            <span className={available ? "status-dot on" : "status-dot"} />
            <span>{label}</span>
            <strong>{available ? "Available" : "Not saved"}</strong>
          </div>
        ))}
      </div>
    </section>
  );
}

function AuditSection({ site }: { readonly site: SiteResponse }): React.ReactNode {
  const audit = site.audit;
  return (
    <section className="section-block" id="site-health">
      <PanelHeading eyebrow="Site health" title="Latest public audit" />
      {audit === undefined ? (
        <EmptyPanel text="Run an audit with --save to display crawl evidence here." />
      ) : (
        <div className="panel table-panel">
          <div className="table-meta">
            <span>{audit.summary.pages} pages</span>
            <span>{audit.summary.sitemap_urls} sitemap URLs</span>
            {audit.performance.available ? (
              <span>Worst LCP {(audit.performance.summary.worst_lcp_ms / 1000).toFixed(1)}s</span>
            ) : null}
          </div>
          {audit.findings.length === 0 ? (
            <div className="clean-state">
              <span>✓</span>
              <div>
                <strong>No deterministic audit actions</strong>
                <p>The latest saved crawl completed without failures or warnings.</p>
              </div>
            </div>
          ) : (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Priority</th>
                    <th>Area</th>
                    <th>Issue</th>
                    <th>URL</th>
                  </tr>
                </thead>
                <tbody>
                  {audit.findings.slice(0, 8).map((finding, index) => (
                    <tr key={`${finding.check}-${finding.url ?? ""}-${index}`}>
                      <td>
                        <span className={`priority ${finding.priority}`}>{finding.priority}</span>
                      </td>
                      <td>{finding.category}</td>
                      <td>{finding.check}</td>
                      <td className="url-cell">{compactURL(finding.url)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </section>
  );
}

function SearchSection({ site }: { readonly site: SiteResponse }): React.ReactNode {
  const gscRows = site.gsc?.striking_distance ?? [];
  const providerRows = site.search?.ranked_keywords ?? [];
  return (
    <section className="section-block" id="search">
      <PanelHeading eyebrow="Search" title="Keyword opportunities" />
      {gscRows.length === 0 && providerRows.length === 0 ? (
        <EmptyPanel text="Save a Search Console or search-opportunity run to populate this table." />
      ) : (
        <div className="panel table-panel">
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Keyword</th>
                  <th>Position</th>
                  <th>Impressions / volume</th>
                  <th>Clicks</th>
                  <th>Page</th>
                </tr>
              </thead>
              <tbody>
                {gscRows.slice(0, 10).map((row) => (
                  <tr key={`${row.query}-${row.page}`}>
                    <td className="strong-cell">{row.query}</td>
                    <td>{row.position.toFixed(1)}</td>
                    <td>{formatCompact(row.impressions)}</td>
                    <td>{formatCompact(row.clicks)}</td>
                    <td className="url-cell">{compactURL(row.page)}</td>
                  </tr>
                ))}
                {gscRows.length === 0
                  ? providerRows.slice(0, 10).map((row) => (
                      <tr key={`${row.keyword}-${row.url}`}>
                        <td className="strong-cell">{row.keyword}</td>
                        <td>{row.position}</td>
                        <td>{formatCompact(row.search_volume)}</td>
                        <td>—</td>
                        <td className="url-cell">{compactURL(row.url)}</td>
                      </tr>
                    ))
                  : null}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </section>
  );
}

function BacklinkSection({ site }: { readonly site: SiteResponse }): React.ReactNode {
  const domains = site.backlinks?.referring_domains ?? [];
  return (
    <section className="section-block" id="backlinks">
      <PanelHeading eyebrow="Authority" title="Top referring domains" />
      {site.backlinks === undefined ? (
        <EmptyPanel text="Run backlink analysis to display authority evidence here." />
      ) : (
        <div className="panel table-panel">
          <div className="table-meta">
            <span>{site.backlinks.backlink_summary.referring_domains} referring domains</span>
            <span>{site.backlinks.backlink_summary.backlinks} backlinks</span>
            <span>DFS rank {site.backlinks.backlink_summary.dataforseo_rank}</span>
          </div>
          {domains.length === 0 ? (
            <EmptyPanel text="No referring-domain rows were returned in the saved snapshot." />
          ) : (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Domain</th>
                    <th>DFS rank</th>
                    <th>Backlinks</th>
                    <th>Spam score</th>
                  </tr>
                </thead>
                <tbody>
                  {domains.slice(0, 10).map((domain) => (
                    <tr key={domain.domain}>
                      <td className="strong-cell">{domain.domain}</td>
                      <td>{domain.dataforseo_rank}</td>
                      <td>{domain.backlinks}</td>
                      <td>{domain.backlinks_spam_score}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </section>
  );
}

function RankingSection({ ranking }: { readonly ranking: Ranking | undefined }): React.ReactNode {
  return (
    <section className="section-block" id="rankings">
      <PanelHeading eyebrow="Rank tracking" title="Tracked positions" />
      {ranking === undefined ? (
        <EmptyPanel text="Add tracked keywords to display current positions." />
      ) : (
        <div className="panel table-panel">
          <div className="table-meta">
            <span>{ranking.summary.tracked_keywords} keywords</span>
            <span>{ranking.summary.ranking} ranking</span>
            <span>{ranking.summary.improved} improved</span>
            <span>{ranking.summary.declined} declined</span>
          </div>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Position</th>
                  <th>Change</th>
                  <th>Keyword</th>
                  <th>Device</th>
                  <th>Ranking page</th>
                </tr>
              </thead>
              <tbody>
                {ranking.rows.map((row) => (
                  <tr key={`${row.keyword}-${row.device}`}>
                    <td className="position-cell">
                      {row.observed ? (row.position ?? `>${ranking.config.serp_depth}`) : "—"}
                    </td>
                    <td>
                      <span className={`change ${row.change}`}>{row.change}</span>
                    </td>
                    <td className="strong-cell">{row.keyword}</td>
                    <td>{row.device}</td>
                    <td className="url-cell">{compactURL(row.ranking_url)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </section>
  );
}

function PanelHeading({
  eyebrow,
  title,
}: {
  readonly eyebrow: string;
  readonly title: string;
}): React.ReactNode {
  return (
    <div className="panel-heading">
      <p className="eyebrow">{eyebrow}</p>
      <h3>{title}</h3>
    </div>
  );
}

function EmptyPanel({ text }: { readonly text: string }): React.ReactNode {
  return (
    <div className="empty-panel">
      <span>＋</span>
      <p>{text}</p>
    </div>
  );
}

function LoadingState(): React.ReactNode {
  return (
    <main className="state-page">
      <div className="loader-mark">S</div>
      <h1>Loading SEO evidence</h1>
      <p>Reading the local audit database.</p>
    </main>
  );
}

function EmptyState(): React.ReactNode {
  return (
    <main className="state-page">
      <div className="loader-mark">S</div>
      <h1>No saved sites yet</h1>
      <p>Save an audit, run backlink analysis, or add tracked keywords, then refresh this dashboard.</p>
    </main>
  );
}

function ErrorState({ message }: { readonly message: string }): React.ReactNode {
  return (
    <main className="state-page">
      <div className="loader-mark error">!</div>
      <h1>Dashboard unavailable</h1>
      <p>{message}</p>
    </main>
  );
}

function formatDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "No saved evidence";
  }
  return new Intl.DateTimeFormat("en-GB", {
    day: "numeric",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

function formatOptional(value: number | undefined): string {
  return value === undefined ? "—" : new Intl.NumberFormat("en-GB").format(value);
}

function formatCompact(value: number | undefined): string {
  if (value === undefined) {
    return "—";
  }
  return new Intl.NumberFormat("en-GB", {
    notation: value >= 1_000 ? "compact" : "standard",
    maximumFractionDigits: 1,
  }).format(value);
}

function compactURL(value: string | undefined): string {
  if (value === undefined || value === "") {
    return "—";
  }
  try {
    const parsed = new URL(value);
    return `${parsed.hostname}${parsed.pathname === "/" ? "" : parsed.pathname}`;
  } catch {
    return value;
  }
}
