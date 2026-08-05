# byz-search

Go search API for Byzantine: **JWT → Solr** with forced org (and optional tenant) filters, plus Kafka analytics.

No database. Saved searches live in clients / profile services.

| Setting | Default |
|---------|---------|
| HTTP | `8099` |
| Solr | `http://127.0.0.1:8983` collection `byz` |
| Kafka topic | `byz.search.query` |

## API

### Search (snippets)

```http
GET /api/v1/search?q=invoice&page=0&size=20
Authorization: Bearer <IAM JWT>
```

List results return **snippets only** (not full bodies).

### Document (full indexed text)

```http
GET /api/v1/documents/{id}?organizationId=<org>&userId=<user>
Authorization: Bearer <IAM JWT>
```

Returns the full Solr-stored `content` field for one document (extracted text at index time).  
Capped by `DOCUMENT_MAX_CHARS` (default 200000); response includes `truncated`.  
Same org/admin override and optional user visibility as search (shared-or-mine when `userId` set).

Optional query params:

| Param | Notes |
|-------|--------|
| `tenantId` | Opt-in tenant filter. Non-admin: must match token `tenant_id` if present. Platform admin (see below) may pass any. |
| `organizationId` | Search as this org. Only platform admin (or unset `BYZ_ADMIN_ORGANIZATION_ID` in local/dev). |
| `userId` | Visibility: docs with **no** `user_id` (shared) **or** `user_id` = this user. Non-admin may only use self. |

Requires JWT claim `organization_id`. Solr always applies:

```text
fq=organization_id:"<org from token or organizationId override>"
```

and optionally `tenant_id` / user visibility filters when present.

**Platform admin / byz-agent:** set `BYZ_ADMIN_ORGANIZATION_ID` to the Byzantine platform org UUID. The `byz-agent` confidential client lives in that org; agent passes the chat run’s org (and tenant/user) so multi-tenant search is correct.

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
| `code_tokens` | strings (multi) | path/API snippets e.g. `/notifications/{id}` |
| `organization_id` | string | **required** filter |
| `tenant_id` | string | optional |
| `user_id` | string | optional |
| `source` | string | e.g. `file-service`, `onedrive` |
| `path` | string | display path |
| `tags` | strings | multi-valued ok |

One-time (if schemaless did not create `code_tokens` as multi-string):

```bash
curl -s -X POST 'http://127.0.0.1:8983/solr/byz/schema' \
  -H 'Content-type: application/json' \
  -d '{"add-field":{"name":"code_tokens","type":"strings","stored":true,"indexed":true,"multiValued":true}}'
```

Re-index (re-upload or republish `search.index`) after adding the field / deploying ingest.

## Query behavior

- Plain words → contains on `title` / `content` / `path` / `code_tokens`
- Path-like (`/`, `{`, `}`) → also substring-match on `code_tokens` (tokenization-safe)
- Explicit Solr syntax (`*`, `:`, `AND`, …) is passed through (except path-like `{…}` queries)

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
| `BYZ_ADMIN_ORGANIZATION_ID` | empty = allow org override (local); set to Byz org UUID in prod for agent |
| `DOCUMENT_MAX_CHARS` | `200000` — max body length on `GET /api/v1/documents/{id}` |

## Run

```bash
export SOLR_URL=http://127.0.0.1:8983
export IAM_JWKS_URL=http://127.0.0.1:8082/.well-known/jwks.json
go run .
```

## Gateway

Expose via `api.byzantineapp.dev/search/**` → this service (StripPrefix=1), same pattern as other backends. See `byz-api-gateway` route `search`.
