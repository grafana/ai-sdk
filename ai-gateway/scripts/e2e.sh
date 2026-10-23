#!/usr/bin/env bash
# Runs the Playwright tests against the built plugin package in the
# create-plugin Grafana environment, then stops the environment.
set -euo pipefail

gateway=$(git rev-parse --show-toplevel)/ai-gateway
grafana_url=http://localhost:3000
startup_attempts=90

fail() { echo "$*" >&2; exit 1; }

[[ -f "$gateway/dist/plugin.json" ]] || fail "build the plugin package first: mise run build-ai-gateway-plugin"
cd "$gateway"

stop() {
  local status=$?
  if [[ $status -ne 0 ]]; then
    docker compose logs grafana | tail -n 200 || true
  fi
  docker compose down --volumes || true
  exit "$status"
}
trap stop EXIT

docker compose up --detach --build
for ((attempt = 0; attempt < startup_attempts; attempt++)); do
  curl -fsS "$grafana_url/api/health" >/dev/null 2>&1 && break
  sleep 2
done
curl -fsS "$grafana_url/api/health" >/dev/null || fail "Grafana did not become healthy"

pnpm exec playwright install chromium ${CI:+--with-deps}
pnpm e2e
