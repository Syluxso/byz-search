#!/bin/bash
set -euo pipefail
export PORT="${PORT:-8099}"
export BIND="${BIND:-127.0.0.1}"
export SOLR_URL="${SOLR_URL:-http://127.0.0.1:8983}"
export SOLR_COLLECTION="${SOLR_COLLECTION:-byz}"
export IAM_JWKS_URL="${IAM_JWKS_URL:-https://iam.byzantineapp.dev/.well-known/jwks.json}"
export KAFKA_BOOTSTRAP="${KAFKA_BOOTSTRAP:-127.0.0.1:9092}"
export BYZ_KAFKA_ENABLED="${BYZ_KAFKA_ENABLED:-true}"
export KAFKA_TOPIC_SEARCH_QUERY="${KAFKA_TOPIC_SEARCH_QUERY:-byz.search.query}"

exec /opt/services/byz-search/byz-search
