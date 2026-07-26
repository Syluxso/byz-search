# byz-search

Go search API for Byzantine: **JWT → Solr** with forced org (and optional tenant) filters, plus Kafka analytics.

No database. Saved searches live in clients / profile services.

| Setting | Default |
|---------|---------|
| HTTP | `8099` |
| Solr | `http://127.0.0.1:8983` collection `byz` |
| Kafka topic | `byz.search.query` |

## API

```http
GET /api/v1/search?q=invoice&page=0&size=20
Authorization: Bearer <IAM JWT>
```

Optional: `tenantId` (must match token `tenant_id` when the token has one).

Requires JWT claim `organization_id`. Search always applies:

```text
fq=organization_id:"<org from token>"
```

and optionally `tenant_id` when present.

### Response

```json
{
  "q": "invoice",
  "page": 0,
  "size": 20,
  "total": 12,
  "durationMs": 18,
  "hits": [
    {
      "id": "file-uuid",
      "title": "Q1 invoice",
      "snippet": "...matched <em>invoice</em>...",
      "score": 2.1,
      "source": "file-service",
      "path": "/docs/q1.pdf",
      "organizationId": "...",
      "tenantId": "..."
    }
  ]
}
```

Limits: `q` required (max 1000 chars), `size` capped (`SEARCH_MAX_SIZE`, default 50).

## Solr schema (Ingest contract)

Create collection `byz` (or set `SOLR_COLLECTION`) with fields:

| Field | Type | Notes |
|-------|------|--------|
| `id` | string | document id (file id, etc.) |
| `title` | text | |
| `content` | text | full text for search + highlighting |
| `organization_id` | string | **required** filter |
| `tenant_id` | string | optional |
| `user_id` | string | optional |
| `source` | string | e.g. `file-service`, `onedrive` |
| `path` | string | display path |
| `tags` | strings | multi-valued ok |

Example (managed schema via Solr API or configset) — keep names exact so Ingest and Search stay aligned.

## Query behavior

Plain queries are treated as **contains** matches over `title`, `content`, and `path`
(`*term*`, `allowLeadingWildcard`). Explicit Solr syntax (`*`, `:`, `AND`, …) is passed through.

Optional query params:

| Param | Notes |
|-------|--------|
| `tenantId` | Opt-in tenant filter (not applied from JWT by default) |
| `organizationId` | Platform-admin override when `BYZ_ADMIN_ORGANIZATION_ID` matches token org (or unset in dev) |

## Analytics

Each search (success or Solr failure) best-effort publishes `search.query` to **`byz.search.query`** (key = `organizationId`).

Contract: `projects/events-service/docs/EVENTS.md`.

## Health / admin

- `GET /actuator/health` — UP only if Solr ping succeeds  
- `GET /healthz` — process liveness (no Solr)
- `GET /api/v1/admin/logs` — in-memory log tail (JWT; Admin Logs UI)

## Config

| Env | Default |
|-----|---------|
| `PORT` | `8099` |
| `BIND` | `0.0.0.0` |
| `SOLR_URL` | `http://127.0.0.1:8983` |
| `SOLR_COLLECTION` | `byz` |
| `IAM_JWKS_URL` | `https://iam.byzantineapp.dev/.well-known/jwks.json` |
| `KAFKA_BOOTSTRAP` | `127.0.0.1:9092` |
| `BYZ_KAFKA_ENABLED` | `true` |
| `KAFKA_TOPIC_SEARCH_QUERY` | `byz.search.query` |
| `SEARCH_MAX_SIZE` | `50` |

## Run

```bash
export SOLR_URL=http://127.0.0.1:8983
export IAM_JWKS_URL=http://127.0.0.1:8082/.well-known/jwks.json
go run .
```

## Gateway

Expose via `api.byzantineapp.dev/search/**` → this service (StripPrefix=1), same pattern as other backends. See `byz-api-gateway` route `search`.
