#!/usr/bin/env bash
# Runs the Playwright tests against the built plugin package in the
# create-plugin Grafana environment, with the end-to-end overrides, their fake
# provider and a proxy for the native provider endpoints, then stops the
# environment. The proxy's certificate is generated for each run and removed
# with the environment.
set -euo pipefail

gateway=$(git rev-parse --show-toplevel)/ai-gateway
grafana_url=http://localhost:3000
startup_attempts=90
catalog_key_sentinel=e2e-catalog-sentinel-key
dump_lines=200

fail() { echo "$*" >&2; exit 1; }

[[ -f "$gateway/dist/plugin.json" ]] || fail "build the plugin package first: mise run build-ai-gateway-plugin"
cd "$gateway"
compose=(docker compose -f docker-compose.yaml -f docker-compose.e2e.yaml)

E2E_NATIVE_PROXY_CERTIFICATES=$(mktemp -d)
export E2E_NATIVE_PROXY_CERTIFICATES

# dump_failure prints, for the whole run, the plugin's log lines and Grafana's
# lines about catalog storage and admission, each bounded, then Grafana's last
# lines.
dump_failure() {
  local log
  log=$("${compose[@]}" logs --no-color grafana 2>&1) || true
  echo "--- plugin log (logger=plugin.grafana-aigateway-app, first $dump_lines lines)"
  grep 'logger=plugin.grafana-aigateway-app' <<<"$log" | head -n "$dump_lines" || true
  echo "--- catalog storage and admission (first $dump_lines lines)"
  grep -v 'logger=plugin.grafana-aigateway-app' <<<"$log" | grep -i -E 'modelcatalog|admission|securevalue|inline secure|apistore' | head -n "$dump_lines" || true
  echo "--- Grafana log (last $dump_lines lines)"
  tail -n "$dump_lines" <<<"$log"
}

stop() {
  local status=$?
  if [[ $status -ne 0 ]]; then
    dump_failure
    "${compose[@]}" logs --no-color native-proxy | tail -n "$dump_lines" || true
  fi
  "${compose[@]}" down --volumes || true
  rm -rf "$E2E_NATIVE_PROXY_CERTIFICATES"
  exit "$status"
}
trap stop EXIT

openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 1 \
  -subj "/CN=AI Gateway end-to-end native provider" \
  -addext "subjectAltName=DNS:api.anthropic.com,DNS:api.openai.com" \
  -addext "basicConstraints=critical,CA:TRUE" \
  -addext "keyUsage=critical,digitalSignature,keyCertSign" \
  -addext "extendedKeyUsage=serverAuth" \
  -keyout "$E2E_NATIVE_PROXY_CERTIFICATES/key.pem" -out "$E2E_NATIVE_PROXY_CERTIFICATES/cert.pem" 2>/dev/null ||
  fail "generating the native proxy certificate needs openssl"
chmod 755 "$E2E_NATIVE_PROXY_CERTIFICATES"
chmod 644 "$E2E_NATIVE_PROXY_CERTIFICATES/key.pem" "$E2E_NATIVE_PROXY_CERTIFICATES/cert.pem"

"${compose[@]}" up --detach --build
for ((attempt = 0; attempt < startup_attempts; attempt++)); do
  curl -fsS "$grafana_url/api/health" >/dev/null 2>&1 && break
  sleep 2
done
curl -fsS "$grafana_url/api/health" >/dev/null || fail "Grafana did not become healthy"

pnpm exec playwright install chromium ${CI:+--with-deps}
pnpm e2e
if "${compose[@]}" logs grafana | grep 'logger=plugin.grafana-aigateway-app' | grep -F -- "$catalog_key_sentinel" >/dev/null; then
  fail "the plugin logged a catalog key"
fi
