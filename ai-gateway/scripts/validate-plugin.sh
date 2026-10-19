#!/usr/bin/env bash
# Validates the built plugin package in ai-gateway/dist: its exact contents and
# the pinned Grafana plugin validator.
set -euo pipefail

validator_version=0.51.0
declare -A validator_sha256=(
  [linux_amd64]=686c2388a3baf3d5f59a2e603c7f42b4a832ee83fc418f50d469e72f72b2a5ae
  [linux_arm64]=1205a18f2d189d9dcf8795d301f9410841a726c8b032e90435afa393a1ef4427
  [darwin_amd64]=3319392a8bf63baf9732276c1fd417f192e02c8770965374c16e728f514f0582
  [darwin_arm64]=6fe2b0cc29faf2de43cd7cc8e21cc1618d4433f89183767fbab77d04513ebc16
)
plugin_id=grafana-aigateway-app
expected_entries=(
  CHANGELOG.md LICENSE README.md SOURCE_REVISION aigateway-manifest.json go_plugin_build_manifest
  gpx_aigateway_linux_amd64 gpx_aigateway_linux_arm64 img licenses module.js module.js.map plugin.json
)

repo_root=$(git rev-parse --show-toplevel)
dist=$repo_root/ai-gateway/dist

fail() { echo "$*" >&2; exit 1; }

[[ -d "$dist" ]] || fail "build the plugin package first: mise run build-ai-gateway-plugin"
actual=$(find "$dist" -mindepth 1 -maxdepth 1 -printf '%f\n' | sort)
expected=$(printf '%s\n' "${expected_entries[@]}" | sort)
[[ "$actual" == "$expected" ]] || fail "unexpected plugin package contents:"$'\n'"$(diff <(echo "$expected") <(echo "$actual") || true)"
[[ $(jq -r .id "$dist/plugin.json") == "$plugin_id" ]] || fail "plugin.json does not declare $plugin_id"

platform="$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')"
checksum=${validator_sha256[$platform]:-}
[[ -n "$checksum" ]] || fail "no pinned plugin validator for $platform"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
archive=plugin-validator_${validator_version}_${platform}.tar.gz
curl -fsSL -o "$work/$archive" "https://github.com/grafana/plugin-validator/releases/download/plugin-validator/v${validator_version}/$archive"
echo "$checksum  $work/$archive" | sha256sum -c --quiet
tar -xzf "$work/$archive" -C "$work" plugincheck2

mkdir -p "$work/package"
cp -R "$dist" "$work/package/$plugin_id"
(cd "$work/package" && python3 -m zipfile -c "$work/$plugin_id.zip" "$plugin_id")
"$work/plugincheck2" "$work/$plugin_id.zip"
