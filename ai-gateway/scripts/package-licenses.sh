#!/usr/bin/env bash
# Adds license, notice and source attribution files to the built plugin package
# in ai-gateway/dist: the Gateway and SDK licenses, a per-platform inventory of
# every Go module linked into each backend with its license and notice files,
# and the licenses of node modules bundled into the frontend.
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
gateway=$repo_root/ai-gateway
dist=$gateway/dist
licenses=$dist/licenses
revision=${AI_GATEWAY_VCS_REF:-local-unverified}

fail() { echo "$*" >&2; exit 1; }

[[ -f "$dist/plugin.json" && -f "$dist/module.js" ]] || fail "build the plugin package before packaging licenses"
rm -rf "$licenses"
mkdir -p "$licenses/grafana-ai-gateway" "$licenses/grafana-ai-sdk"
cp "$gateway/LICENSE" "$gateway/NOTICE" "$licenses/grafana-ai-gateway/"
cp "$repo_root/LICENSE" "$repo_root/NOTICE" "$licenses/grafana-ai-sdk/"
if [[ "$revision" == local-unverified ]]; then
  printf 'local-unverified: development build from an unverified working tree\n' >"$dist/SOURCE_REVISION"
else
  [[ "$revision" =~ ^[0-9a-f]{40}$ ]] || fail "AI_GATEWAY_VCS_REF must be a full commit SHA"
  printf '%s\n' "$revision" >"$dist/SOURCE_REVISION"
fi

copy_license_files() {
  local source=$1 destination=$2 file
  mkdir -p "$destination"
  for file in "$source"/LICENSE* "$source"/LICENCE* "$source"/NOTICE* "$source"/COPYING*; do
    [[ -f "$file" ]] || continue
    cp "$file" "$destination/"
  done
}

for platform in linux/amd64 linux/arm64; do
  goos=${platform%/*} goarch=${platform#*/}
  output=$licenses/backend-${goos}_${goarch}
  mkdir -p "$output/modules"
  modules=$(cd "$gateway" && GOFLAGS="-mod=readonly" GOOS=$goos GOARCH=$goarch CGO_ENABLED=0 \
    go list -deps -tags arrow_json_stdlib \
    -f '{{with .Module}}{{.Path}}{{"\t"}}{{if or .Replace (not .Version)}}local{{else}}{{.Version}}{{end}}{{"\t"}}{{if .Sum}}{{.Sum}}{{else}}-{{end}}{{"\t"}}{{.Dir}}{{end}}' \
    ./cmd/grafana-ai-gateway | sort -u)
  : >"$output/THIRD_PARTY_MODULES.txt"
  while IFS=$'\t' read -r module version checksum directory; do
    [[ -n "$module" ]] || continue
    if [[ "$version" == local ]]; then
      case "$directory" in "$repo_root" | "$repo_root"/*) ;; *) fail "unexpected local module: $module" ;; esac
      attribution=$revision
      license_dir=$repo_root
      [[ "$directory" != "$gateway" ]] || license_dir=$gateway
    else
      [[ "$checksum" != - ]] || fail "missing module checksum: $module"
      attribution=$version
      license_dir=$directory
    fi
    printf '%s\t%s\t%s\n' "$module" "$attribution" "$checksum" >>"$output/THIRD_PARTY_MODULES.txt"
    copy_license_files "$license_dir" "$output/modules/${module}@${attribution}"
  done <<<"$modules"
done

frontend=$licenses/frontend
mkdir -p "$frontend"
: >"$frontend/THIRD_PARTY_MODULES.txt"
packages=$(jq -r '.sources[]' "$dist"/*.js.map | sed -nE 's#.*node_modules/((@[^/]+/)?[^/]+)/.*#\1#p; s#.*webpack/runtime/.*#webpack#p' | sort -u)
while IFS= read -r package; do
  [[ -n "$package" ]] || continue
  directory=$(cd "$gateway" && node -e 'const path = require("node:path"); console.log(path.dirname(require.resolve(process.argv[1] + "/package.json")))' "$package")
  version=$(jq -r .version "$directory/package.json")
  printf '%s\t%s\n' "$package" "$version" >>"$frontend/THIRD_PARTY_MODULES.txt"
  copy_license_files "$directory" "$frontend/modules/${package}@${version}"
done <<<"$packages"
