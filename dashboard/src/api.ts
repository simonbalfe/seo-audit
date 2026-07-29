import { z } from "zod";

const SiteSummarySchema = z.object({
  target: z.string(),
  last_updated: z.string(),
  has_audit: z.boolean(),
  has_gsc: z.boolean(),
  has_search: z.boolean(),
  has_backlinks: z.boolean(),
  ranking_trackers: z.number(),
});

const FindingSchema = z.object({
  category: z.string(),
  check: z.string(),
  status: z.string(),
  priority: z.string(),
  url: z.string().optional(),
  evidence: z.string(),
  fix: z.string().optional(),
});

const AuditPageSchema = z.object({
  url: z.string(),
  status_code: z.number(),
  indexable: z.boolean(),
  indexability: z.string().optional(),
  title: z.string().optional(),
  word_count: z.number(),
  depth: z.number(),
  inlinks: z.number(),
  findings: z.number(),
});

const AuditSchema = z.object({
  snapshot_id: z.number(),
  retrieved_at: z.string(),
  start_url: z.string(),
  duration_ms: z.number(),
  limit_reached: z.boolean(),
  summary: z.object({
    pages: z.number(),
    indexable: z.number(),
    non_indexable: z.number(),
    failures: z.number(),
    warnings: z.number(),
    internal_links: z.number(),
    external_links: z.number(),
    broken_internal_links: z.number(),
    redirected_internal_links: z.number(),
    sitemap_urls: z.number(),
  }),
  performance: z.object({
    available: z.boolean(),
    profile: z.string(),
    summary: z.object({
      pages: z.number(),
      errors: z.number(),
      worst_lcp_ms: z.number(),
      worst_cls: z.number(),
      worst_tbt_ms: z.number(),
      worst_ttfb_ms: z.number(),
      max_transfer_bytes: z.number(),
    }),
  }),
  findings: z.array(FindingSchema),
  pages: z.array(AuditPageSchema),
});

const GSCMetricSchema = z.object({
  query: z.string(),
  page: z.string(),
  clicks: z.number(),
  impressions: z.number(),
  ctr: z.number(),
  position: z.number(),
});

const GSCSchema = z.object({
  source: z.string(),
  site_url: z.string(),
  start_date: z.string(),
  end_date: z.string(),
  retrieved_at: z.string(),
  summary: z.object({
    rows: z.number(),
    returned_clicks: z.number(),
    returned_impressions: z.number(),
    returned_ctr: z.number(),
    weighted_position: z.number(),
  }),
  striking_distance: z.array(GSCMetricSchema),
});

const RankedKeywordSchema = z.object({
  keyword: z.string(),
  position: z.number(),
  previous_position: z.number().optional(),
  url: z.string(),
  search_volume: z.number(),
  difficulty: z.number().optional(),
  intent: z.string().optional(),
  estimated_traffic: z.number(),
});

const KeywordIdeaSchema = z.object({
  keyword: z.string(),
  search_volume: z.number(),
  difficulty: z.number().optional(),
  cpc: z.number().optional(),
  intent: z.string().optional(),
});

const SearchSchema = z.object({
  source: z.string(),
  target: z.string(),
  retrieved_at: z.string(),
  cost_usd: z.number(),
  organic_visibility: z.object({
    keywords: z.number(),
    estimated_traffic: z.number(),
    position_1: z.number(),
    positions_2_to_3: z.number(),
    positions_4_to_10: z.number(),
    positions_11_to_20: z.number(),
    positions_21_to_100: z.number(),
    new: z.number(),
    up: z.number(),
    down: z.number(),
    lost: z.number(),
  }),
  ranked_keywords: z.array(RankedKeywordSchema).optional(),
  keyword_ideas: z.array(KeywordIdeaSchema).optional(),
});

const ReferringDomainSchema = z.object({
  domain: z.string(),
  dataforseo_rank: z.number(),
  backlinks: z.number(),
  backlinks_spam_score: z.number(),
  referring_pages: z.number(),
  nofollow_referring_pages: z.number(),
});

const BacklinksSchema = z.object({
  source: z.string(),
  target: z.string(),
  retrieved_at: z.string(),
  cost_usd: z.number(),
  backlink_summary: z.object({
    dataforseo_rank: z.number(),
    target_spam_score: z.number(),
    backlinks: z.number(),
    referring_domains: z.number(),
    broken_backlinks: z.number(),
  }),
  referring_domains: z.array(ReferringDomainSchema).optional(),
});

const RankRowSchema = z.object({
  keyword: z.string(),
  device: z.string(),
  observed: z.boolean(),
  position: z.number().optional(),
  previous_position: z.number().optional(),
  previous_observed: z.boolean(),
  ranking_url: z.string().optional(),
  serp_features: z.array(z.string()).optional(),
  change: z.string(),
});

const RankingSchema = z.object({
  config: z.object({
    id: z.number(),
    target: z.string(),
    location: z.string(),
    language: z.string(),
    devices: z.string(),
    serp_depth: z.number(),
  }),
  latest_run: z
    .object({
      id: z.number(),
      status: z.string(),
      successful_tasks: z.number(),
      requested_tasks: z.number(),
      cost_usd: z.number(),
      started_at: z.string(),
    })
    .optional(),
  summary: z.object({
    tracked_keywords: z.number(),
    tracked_tasks: z.number(),
    checked: z.number(),
    not_checked: z.number(),
    ranking: z.number(),
    not_ranking: z.number(),
    top_3: z.number(),
    top_10: z.number(),
    improved: z.number(),
    declined: z.number(),
    new: z.number(),
    lost: z.number(),
    stable: z.number(),
    uncompared: z.number(),
  }),
  rows: z.array(RankRowSchema),
});

export const SitesResponseSchema = z.object({
  generated_at: z.string(),
  sites: z.array(SiteSummarySchema),
});

export const SiteResponseSchema = z.object({
  generated_at: z.string(),
  target: z.string(),
  last_updated: z.string(),
  audit: AuditSchema.optional(),
  gsc: GSCSchema.optional(),
  search: SearchSchema.optional(),
  backlinks: BacklinksSchema.optional(),
  rankings: z.array(RankingSchema),
  warnings: z.array(z.string()).optional(),
});

export type SiteSummary = z.infer<typeof SiteSummarySchema>;
export type SitesResponse = z.infer<typeof SitesResponseSchema>;
export type SiteResponse = z.infer<typeof SiteResponseSchema>;
export type Ranking = z.infer<typeof RankingSchema>;

async function request(path: string): Promise<unknown> {
  const response = await fetch(path, {
    headers: {
      Accept: "application/json",
    },
  });
  const payload: unknown = await response.json();
  if (!response.ok) {
    if (
      typeof payload === "object" &&
      payload !== null &&
      "error" in payload &&
      typeof payload.error === "string"
    ) {
      throw new Error(payload.error);
    }
    throw new Error(`Dashboard request failed with HTTP ${response.status}`);
  }
  return payload;
}

export async function loadSites(signal: AbortSignal): Promise<SitesResponse> {
  const response = await fetch("/api/v1/sites", {
    signal,
    headers: {
      Accept: "application/json",
    },
  });
  const payload: unknown = await response.json();
  if (!response.ok) {
    throw new Error(`Could not load sites: HTTP ${response.status}`);
  }
  return SitesResponseSchema.parse(payload);
}

export async function loadSite(target: string): Promise<SiteResponse> {
  const payload = await request(`/api/v1/sites/${encodeURIComponent(target)}`);
  return SiteResponseSchema.parse(payload);
}
