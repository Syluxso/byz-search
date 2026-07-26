# Deploy byz-search on Linode

## Prereqs

1. Solr installed and listening (e.g. `127.0.0.1:8983`)
2. Collection `byz` created with fields from README
3. Topic `byz.search.query` created (events bootstrap or manual)

## Env

```bash
export PORT=8099
export BIND=127.0.0.1
export SOLR_URL=http://127.0.0.1:8983
export SOLR_COLLECTION=byz
export IAM_JWKS_URL=https://iam.byzantineapp.dev/.well-known/jwks.json
export KAFKA_BOOTSTRAP=127.0.0.1:9092
export BYZ_KAFKA_ENABLED=true
```

| Item | Value |
|------|--------|
| Deploy dir | `/opt/services/byz-search` |
| Binary | `byz-search` |
| Supervisor | `byz-search` |

## Verify

```bash
curl -s http://127.0.0.1:8099/actuator/health
# With JWT:
# curl -s 'http://127.0.0.1:8099/api/v1/search?q=test' -H "Authorization: Bearer $TOKEN"
```

Nginx: either `search.byzantineapp.dev` → `:8099`, or only via gateway `/search/**`.
