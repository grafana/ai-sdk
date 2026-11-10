#!/usr/bin/env bash
# Runs the Playwright tests against the built plugin package in the
# create-plugin Grafana environment, with the end-to-end overrides and their
# fake provider, then stops the environment.
set -euo pipefail

gateway=$(git rev-parse --show-toplevel)/ai-gateway
grafana_url=http://localhost:3000
startup_attempts=90
catalog_key_sentinel=e2e-catalog-sentinel-key

fail() { echo "$*" >&2; exit 1; }

[[ -f "$gateway/dist/plugin.json" ]] || fail "build the plugin package first: mise run build-ai-gateway-plugin"
cd "$gateway"
compose=(docker compose -f docker-compose.yaml -f docker-compose.e2e.yaml)

stop() {
  local status=$?
  if [[ $status -ne 0 ]]; then
    "${compose[@]}" logs grafana | tail -n 200 || true
  fi
  "${compose[@]}" down --volumes || true
  exit "$status"
}
trap stop EXIT

"${compose[@]}" up --detach --build
for ((attempt = 0; attempt < startup_attempts; attempt++)); do
  curl -fsS "$grafana_url/api/health" >/dev/null 2>&1 && break
  sleep 2
done
curl -fsS "$grafana_url/api/health" >/dev/null || fail "Grafana did not become healthy"

pnpm exec playwright install chromium ${CI:+--with-deps}
pnpm e2e
if "${compose[@]}" logs grafana | grep 'logger=plugin.grafana-aigateway-app' | grep -q "$catalog_key_sentinel"; then
  fail "the plugin logged a catalog key"
fi
