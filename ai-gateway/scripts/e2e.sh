#!/usr/bin/env bash
# Runs the Playwright tests against the built plugin package in the
# create-plugin Grafana environment, with the end-to-end overrides and their
# fake provider, then stops the environment.
set -euo pipefail

gateway=$(git rev-parse --show-toplevel)/ai-gateway
grafana_url=http://localhost:3000
startup_attempts=90
catalog_key_sentinel=e2e-catalog-sentinel-key
middleware_config_sentinel=e2e-middleware-sentinel-value

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
plugin_logs=$("${compose[@]}" logs grafana | grep 'logger=plugin.grafana-aigateway-app' || true)
# The sentinel checks below inspect these lines, so they must not be empty:
# the Gateway logs once whether Grafana gave the plugin API access, which it
# needs to read Middleware resources.
api_access=$(grep 'gateway plugin API access' <<<"$plugin_logs" || true)
[[ -n "$api_access" ]] || fail "the plugin log has no API access record"
grep -q 'available=true' <<<"$api_access" || fail "Grafana gave the plugin no API access: $api_access"
if grep -q "$catalog_key_sentinel" <<<"$plugin_logs"; then
  fail "the plugin logged a catalog key"
fi
if grep -q "$middleware_config_sentinel" <<<"$plugin_logs"; then
  fail "the plugin logged a Middleware config value"
fi
