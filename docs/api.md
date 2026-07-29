# REST API

The Go REST API is the execution boundary for SEO Audit. It owns crawling, provider authentication, paid calls, validation, caching, snapshots, and rank-tracking persistence. The CLI is an HTTP client that submits work, follows job progress, retrieves results, and formats them for the terminal.

Start the localhost API and embedded dashboard:

```sh
seoaudit-api
```

The default server URL is `http://127.0.0.1:8787`, with versioned routes under `/api/v1`. The server only accepts a localhost or loopback listen address.

## Endpoint map

| Method | Path | Purpose | Execution |
| --- | --- | --- | --- |
| `GET` | `/api/v1/health` | Liveness | Immediate |
| `GET` | `/api/v1/capabilities` | Limits and provider configuration state | Immediate |
| `POST` | `/api/v1/audits` | Run a public crawl and optional performance checks | Asynchronous job |
| `POST` | `/api/v1/opportunities` | Run explicitly selected GSC and/or DataForSEO search analysis | Asynchronous job |
| `POST` | `/api/v1/backlinks` | Run explicit paid DataForSEO backlink analysis | Asynchronous job |
| `GET` | `/api/v1/jobs/{id}` | Read status and progress events | Immediate |
| `GET` | `/api/v1/jobs/{id}/events` | Read progress events after a cursor | Immediate |
| `GET` | `/api/v1/jobs/{id}/result` | Read a completed operation result | Immediate |
| `DELETE` | `/api/v1/jobs/{id}` | Request job cancellation | Immediate |
| `GET` | `/api/v1/rank-trackers` | List trackers, optionally filtered by target, location, and language | Immediate |
| `POST` | `/api/v1/rank-trackers` | Create or update a tracker and add keywords | Immediate |
| `GET` | `/api/v1/rank-trackers/{id}` | Read a tracker and its latest comparison report | Immediate |
| `PATCH` | `/api/v1/rank-trackers/{id}` | Change device or SERP depth | Immediate |
| `PATCH` | `/api/v1/rank-trackers/{id}/keywords` | Add or remove tracked keywords | Immediate |
| `POST` | `/api/v1/rank-trackers/{id}/checks` | Run an explicit paid rank check | Asynchronous job |
| `GET` | `/api/v1/sites` | List targets with saved evidence | Immediate |
| `GET` | `/api/v1/sites/{target}` | Join the latest saved evidence for one target | Immediate |

The separate `seoaudit-api` server composition mounts the REST handler under `/api/` and the embedded React application under `/`. The dashboard reads the site endpoints and does not trigger collection.

## Asynchronous jobs

Long crawls and provider calls return `202 Accepted` rather than holding an HTTP connection open:

```json
{
  "id": "31e8ce6e51cbf136e8e76496e547b340",
  "kind": "audit",
  "status": "queued",
  "created_at": "2026-07-29T12:00:00Z",
  "status_url": "/api/v1/jobs/31e8ce6e51cbf136e8e76496e547b340",
  "events_url": "/api/v1/jobs/31e8ce6e51cbf136e8e76496e547b340/events",
  "result_url": "/api/v1/jobs/31e8ce6e51cbf136e8e76496e547b340/result"
}
```

Job states are `queued`, `running`, `succeeded`, `failed`, and `cancelled`. `GET /jobs/{id}?after=12` returns only events with a sequence greater than 12. The dedicated events route uses the same `after` cursor and returns:

```json
{
  "events": [],
  "next_after": 12
}
```

A result is available only after completion. Requesting it while a job is active returns `409 Conflict`.

Execution `POST` requests accept an `Idempotency-Key` header. Repeating the same operation with the same key returns the original job and sets `Idempotency-Replayed: true`. Clients must not reuse a key for a different request body. The CLI creates a new key for every invocation.

Jobs and unsaved results are kept in bounded process memory. The API rejects new jobs with `503 Service Unavailable` when capacity is occupied by queued or running work and no completed job can be evicted. Restarting the API removes jobs. Saved reports, paid-provider snapshots, and rank history remain in SQLite.

## Audit request

```http
POST /api/v1/audits
Content-Type: application/json
Idempotency-Key: 84f893e42e074ba98404bb6862a19a25
```

```json
{
  "url": "https://example.com",
  "page_limit": 500,
  "request_timeout_seconds": 30,
  "check_external": true,
  "check_performance": true,
  "save": false
}
```

The result is the complete `audit.SiteReport`. The public operation makes no authenticated or paid calls. `save` controls whether the completed report is written to the API database.

## Opportunity request

```json
{
  "url": "https://example.com",
  "sources": ["gsc", "dataforseo"],
  "gsc": {
    "site_url": "sc-domain:example.com",
    "days": 28,
    "row_limit": 250,
    "save": true
  },
  "dataforseo": {
    "location": "United Kingdom",
    "language": "en",
    "row_limit": 25,
    "cache_ttl_seconds": 21600,
    "refresh": false
  }
}
```

Sources must be explicitly selected. GSC uses server-side Google credentials. DataForSEO uses server-side API credentials and retains its provider cache and snapshot semantics.

## Backlink request

```json
{
  "url": "https://example.com",
  "source": "dataforseo",
  "row_limit": 25,
  "cache_ttl_seconds": 21600,
  "refresh": false
}
```

`source` must explicitly be `dataforseo`. The response preserves provider cost, cache evidence, partial dataset errors, and snapshot identity.

## Rank tracker requests

Create or update the tracker identified by target, location, and language, then add keywords:

```json
{
  "url": "https://example.com",
  "location": "United Kingdom",
  "language": "en",
  "devices": "both",
  "serp_depth": 100,
  "keywords": ["seo audit tool", "technical seo software"]
}
```

Omitting `devices` or `serp_depth` preserves an existing tracker setting and uses the documented defaults when creating one.

Add keywords:

```json
{
  "add": ["seo crawler"]
}
```

Remove keywords while preserving historical observations:

```json
{
  "remove": ["technical seo software"]
}
```

One keyword mutation is accepted per request. A paid check requires:

```json
{
  "source": "dataforseo"
}
```

## Errors

API errors use `application/problem+json`:

```json
{
  "type": "https://seoaudit.local/problems/validation_failed",
  "title": "Bad Request",
  "status": 400,
  "detail": "page_limit must be from 1 to 5000",
  "code": "validation_failed",
  "fields": {
    "page_limit": "out of range"
  }
}
```

The main status codes are:

| Status | Meaning |
| --- | --- |
| `200` | Immediate read or mutation completed |
| `202` | Job accepted or cancellation requested |
| `400` | Invalid request or bounded option |
| `404` | Job, tracker, site, or route not found |
| `409` | Job result requested before completion |
| `415` | Write request is not JSON |
| `503` | In-memory job capacity is occupied |
| `500` | Internal execution or storage failure |

Unknown JSON fields are rejected. Write requests must use `Content-Type: application/json`.

## CLI mapping

| CLI | API calls |
| --- | --- |
| `seoaudit audit` | `POST /audits`, poll `GET /jobs/{id}`, then `GET /jobs/{id}/result` |
| `seoaudit opportunities` | `POST /opportunities`, poll job, fetch result |
| `seoaudit backlinks` | `POST /backlinks`, poll job, fetch result |
| `seoaudit rankings add` | `POST /rank-trackers` |
| `seoaudit rankings remove` | `GET /rank-trackers`, then `PATCH /rank-trackers/{id}/keywords` |
| `seoaudit rankings check` | `GET /rank-trackers`, `POST /rank-trackers/{id}/checks`, poll job, fetch result |
| `seoaudit rankings report` | `GET /rank-trackers` |

`--verbose` prints job events to stderr. `--json` controls terminal formatting only. Database paths and provider credentials are server concerns and are never handled by the proxy commands.

Set a non-default API address with:

```sh
export SEOAUDIT_API_URL="http://127.0.0.1:8787"
```

or:

```sh
seoaudit --api-url http://127.0.0.1:8787 audit https://example.com
```
