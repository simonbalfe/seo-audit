import { StrictMode, useEffect, useRef, useState } from "react";
import type { ReactElement } from "react";
import { createRoot } from "react-dom/client";
import "./styles.css";

type Business = {
  readonly id: number;
  readonly name: string;
  readonly place_id: string;
  readonly site_url: string;
  readonly page_count: number;
  readonly updated_at: string;
};

type Page = {
  readonly url: string;
  readonly final_url: string;
  readonly page_type: string;
  readonly page_type_source: string;
  readonly status_code: number;
  readonly indexable: boolean;
  readonly title: string;
  readonly h1: string;
  readonly depth: number;
  readonly inlinks: number;
  readonly word_count: number;
  readonly issues: number;
  readonly updated_at: string;
};

type AuditJob = {
  readonly id?: string;
  readonly status: "idle" | "running" | "completed" | "failed";
  readonly stage?: string;
  readonly steps?: string;
  readonly place_id?: string;
  readonly logs: readonly string[];
  readonly output_path?: string;
  readonly error?: string;
  readonly started_at?: string;
  readonly finished_at?: string;
};

type Workflow = {
  readonly value: string;
  readonly label: string;
  readonly detail: string;
  readonly stages: readonly string[];
};

const workflows: readonly Workflow[] = [
  { value: "all", label: "Full audit", detail: "Run every check", stages: ["profile", "website", "performance", "visibility", "backlinks", "done"] },
  { value: "website", label: "Website", detail: "Crawl and technical checks", stages: ["profile", "website", "done"] },
  { value: "performance", label: "Performance", detail: "Crawl and mobile lab tests", stages: ["profile", "website", "performance", "done"] },
  { value: "visibility", label: "Visibility", detail: "Keywords, Search, Maps and grid", stages: ["profile", "website", "visibility", "done"] },
  { value: "backlinks", label: "Backlinks", detail: "Domain authority summary", stages: ["profile", "backlinks", "done"] },
  { value: "profile", label: "GBP", detail: "Public profile details", stages: ["profile", "done"] },
];

const stageLabels: Readonly<Record<string, string>> = {
  profile: "GBP",
  website: "Website",
  performance: "Performance",
  visibility: "Visibility",
  backlinks: "Backlinks",
  done: "Complete",
};

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === "object" && value !== null;

const isBusiness = (value: unknown): value is Business =>
  isRecord(value) &&
  typeof value.id === "number" &&
  typeof value.name === "string" &&
  typeof value.place_id === "string" &&
  typeof value.site_url === "string" &&
  typeof value.page_count === "number" &&
  typeof value.updated_at === "string";

const isPage = (value: unknown): value is Page =>
  isRecord(value) &&
  typeof value.url === "string" &&
  typeof value.final_url === "string" &&
  typeof value.page_type === "string" &&
  typeof value.page_type_source === "string" &&
  typeof value.status_code === "number" &&
  typeof value.indexable === "boolean" &&
  typeof value.title === "string" &&
  typeof value.h1 === "string" &&
  typeof value.depth === "number" &&
  typeof value.inlinks === "number" &&
  typeof value.word_count === "number" &&
  typeof value.issues === "number" &&
  typeof value.updated_at === "string";

const isAuditJob = (value: unknown): value is AuditJob =>
  isRecord(value) &&
  (value.status === "idle" || value.status === "running" || value.status === "completed" || value.status === "failed") &&
  Array.isArray(value.logs) &&
  value.logs.every((line) => typeof line === "string");

const fetchData = async (resource: string): Promise<unknown> => {
  const response = await fetch(resource);
  if (!response.ok) {
    throw new Error(`Request failed with ${response.status}`);
  }
  return response.json() as Promise<unknown>;
};

const readableDate = (value: string): string =>
  new Intl.DateTimeFormat("en-GB", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));

export const App = (): ReactElement => {
  const [businesses, setBusinesses] = useState<readonly Business[]>([]);
  const [selectedID, setSelectedID] = useState<number | null>(null);
  const [pages, setPages] = useState<readonly Page[]>([]);
  const [placeID, setPlaceID] = useState("");
  const [limit, setLimit] = useState(50);
  const [timeoutSeconds, setTimeoutSeconds] = useState(30);
  const [checkExternal, setCheckExternal] = useState(true);
  const [performance, setPerformance] = useState(true);
  const [keywords, setKeywords] = useState("");
  const [job, setJob] = useState<AuditJob>({ status: "idle", logs: [] });
  const [query, setQuery] = useState("");
  const [pageType, setPageType] = useState("all");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [refresh, setRefresh] = useState(0);
  const previousJobStatus = useRef(job.status);

  useEffect(() => {
    void fetchData("/api/businesses")
      .then((value) => {
        if (!Array.isArray(value) || !value.every(isBusiness)) {
          throw new Error("The businesses response was invalid");
        }
        setBusinesses(value);
        setSelectedID((current) => current !== null && value.some((item) => item.id === current) ? current : value[0]?.id ?? null);
        setPlaceID((current) => current || value[0]?.place_id || "");
        setLoading(false);
      })
      .catch((cause: unknown) => {
        setError(cause instanceof Error ? cause.message : "Could not load businesses");
        setLoading(false);
      });
  }, [refresh]);

  useEffect(() => {
    if (selectedID === null) {
      setPages([]);
      return;
    }
    setLoading(true);
    setError("");
    void fetchData(`/api/businesses/${selectedID}/pages`)
      .then((value) => {
        if (!Array.isArray(value) || !value.every(isPage)) {
          throw new Error("The pages response was invalid");
        }
        setPages(value);
        setLoading(false);
      })
      .catch((cause: unknown) => {
        setError(cause instanceof Error ? cause.message : "Could not load pages");
        setLoading(false);
      });
  }, [selectedID, refresh]);

  useEffect(() => {
    const updateJob = (): void => {
      void fetchData("/api/audits/current")
        .then((value) => {
          if (!isAuditJob(value)) {
            throw new Error("The audit progress response was invalid");
          }
          setJob(value);
          if (previousJobStatus.current === "running" && value.status === "completed") {
            setRefresh((current) => current + 1);
          }
          previousJobStatus.current = value.status;
        })
        .catch((cause: unknown) => setError(cause instanceof Error ? cause.message : "Could not load audit progress"));
    };
    updateJob();
    const timer = window.setInterval(updateJob, 800);
    return () => window.clearInterval(timer);
  }, []);

  const startAudit = async (steps: string): Promise<void> => {
    setError("");
    const response = await fetch("/api/audits", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        place_id: placeID,
        steps,
        limit,
        timeout_seconds: timeoutSeconds,
        check_external: checkExternal,
        performance,
        keywords: keywords.split(/[\n,]/).map((item) => item.trim()).filter(Boolean),
      }),
    });
    if (!response.ok) {
      throw new Error((await response.text()).trim() || `Request failed with ${response.status}`);
    }
    const value: unknown = await response.json();
    if (!isAuditJob(value)) {
      throw new Error("The audit response was invalid");
    }
    previousJobStatus.current = value.status;
    setJob(value);
  };

  const selected = businesses.find((item) => item.id === selectedID);
  const workflow = workflows.find((item) => item.value === job.steps) ?? workflows[0];
  const stageIndex = workflow?.stages.indexOf(job.stage ?? "") ?? -1;
  const pageTypes = [...new Set(pages.map((item) => item.page_type))].sort();
  const normalizedQuery = query.trim().toLowerCase();
  const visiblePages = pages.filter((item) => {
    const matchesType = pageType === "all" || item.page_type === pageType;
    const matchesQuery = normalizedQuery === "" || `${item.url} ${item.title} ${item.h1}`.toLowerCase().includes(normalizedQuery);
    return matchesType && matchesQuery;
  });
  const indexable = pages.filter((item) => item.indexable).length;
  const withIssues = pages.filter((item) => item.issues > 0).length;

  return (
    <main className="app">
      <header className="topbar">
        <strong>SEO Audit</strong>
        <label className="business-picker">
          <span>Saved business</span>
          <select
            value={selectedID ?? ""}
            onChange={(event) => {
              const id = Number(event.target.value);
              setSelectedID(id);
              const business = businesses.find((item) => item.id === id);
              if (business?.place_id) setPlaceID(business.place_id);
              setPageType("all");
              setQuery("");
            }}
            disabled={businesses.length === 0}
          >
            {businesses.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
        </label>
      </header>

      <section className="runner-panel">
        <div className="runner-form">
          <label className="place-field"><span>Google Place ID</span><input value={placeID} onChange={(event) => setPlaceID(event.target.value)} placeholder="ChIJ…" /></label>
          <label><span>Pages</span><input type="number" min="1" max="5000" value={limit} onChange={(event) => setLimit(event.target.valueAsNumber)} /></label>
          <label><span>Timeout</span><input type="number" min="1" max="120" value={timeoutSeconds} onChange={(event) => setTimeoutSeconds(event.target.valueAsNumber)} /></label>
          <label className="check"><input type="checkbox" checked={checkExternal} onChange={(event) => setCheckExternal(event.target.checked)} /> External links</label>
          <label className="check"><input type="checkbox" checked={performance} onChange={(event) => setPerformance(event.target.checked)} /> Performance in full audit</label>
          <label className="keyword-field"><span>Optional keywords</span><input value={keywords} onChange={(event) => setKeywords(event.target.value)} placeholder="dentist near me, dental implants" /></label>
        </div>
        <div className="workflow-buttons">
          {workflows.map((item) => (
            <button key={item.value} disabled={job.status === "running" || placeID.trim() === ""} onClick={() => void startAudit(item.value).catch((cause: unknown) => setError(cause instanceof Error ? cause.message : "Could not start audit"))}>
              <strong>{job.status === "running" && job.steps === item.value ? "Running…" : item.label}</strong>
              <span>{item.detail}</span>
            </button>
          ))}
        </div>
      </section>

      {job.status !== "idle" && workflow !== undefined && (
        <section className={`job-panel ${job.status}`}>
          <div className="job-heading"><strong>{workflow.label}</strong><span>{job.status === "running" ? stageLabels[job.stage ?? ""] ?? "Starting" : job.status}</span></div>
          <ol className="progress-steps">
            {workflow.stages.map((stage, index) => <li key={stage} className={job.status === "completed" || index < stageIndex ? "complete" : index === stageIndex ? "active" : ""}>{stageLabels[stage]}</li>)}
          </ol>
          {job.error && <p className="job-error">{job.error}</p>}
          {job.output_path && <p className="output-path">Report: {job.output_path}</p>}
          <pre className="logs" aria-live="polite">{job.logs.slice(-40).join("\n") || "Waiting for progress…"}</pre>
        </section>
      )}

      {error !== "" && <div className="notice error">{error}</div>}
      {!loading && businesses.length === 0 && <div className="notice">No saved audits yet.</div>}

      {selected !== undefined && (
        <>
          <section className="summarybar">
            <strong>{selected.name}</strong>
            <a href={selected.site_url} target="_blank" rel="noreferrer">{selected.site_url}</a>
            <span>Updated {readableDate(selected.updated_at)}</span>
            <span><b>{pages.length}</b> pages</span>
            <span><b>{indexable}</b> indexable</span>
            <span><b>{pageTypes.length}</b> types</span>
            <span className={withIssues > 0 ? "has-issues" : ""}><b>{withIssues}</b> with issues</span>
          </section>

          <section className="pages-panel">
            <div className="toolbar">
              <strong>Crawled pages</strong><span>{visiblePages.length} shown</span>
              <div className="filters">
                <input type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search URL, title or H1" aria-label="Search crawled pages" />
                <select value={pageType} onChange={(event) => setPageType(event.target.value)} aria-label="Filter by page type">
                  <option value="all">All page types</option>
                  {pageTypes.map((item) => <option key={item} value={item}>{item}</option>)}
                </select>
              </div>
            </div>
            {loading ? <div className="table-state">Loading pages…</div> : (
              <div className="table-wrap">
                <table>
                  <thead><tr><th>Page</th><th>Type</th><th>Status</th><th>Depth</th><th>Internal links</th><th>Words</th><th>Issues</th></tr></thead>
                  <tbody>{visiblePages.map((item) => (
                    <tr key={item.url}>
                      <td className="page-cell"><a href={item.final_url || item.url} target="_blank" rel="noreferrer">{item.title || item.url}</a><span>{item.url}</span>{item.h1 !== "" && <small>H1: {item.h1}</small>}</td>
                      <td><span className="page-type">{item.page_type || "unknown"}</span><small>{item.page_type_source}</small></td>
                      <td className={item.indexable ? "status-good" : "status-warn"}>{item.status_code} · {item.indexable ? "Indexable" : "Blocked"}</td>
                      <td>{item.depth}</td><td>{item.inlinks}</td><td>{item.word_count.toLocaleString()}</td><td className={item.issues > 0 ? "has-issues" : ""}>{item.issues}</td>
                    </tr>
                  ))}</tbody>
                </table>
                {visiblePages.length === 0 && <div className="table-state">No pages match these filters.</div>}
              </div>
            )}
          </section>
        </>
      )}
    </main>
  );
};

const root = document.getElementById("root");
if (root === null) throw new Error("Dashboard root is missing");
createRoot(root).render(<StrictMode><App /></StrictMode>);
