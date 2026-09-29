# syntax=docker/dockerfile:1.27@sha256:bde3983e9c939224420ddaf6b784cc30e09b035a4dea01f581230c50809f372e

FROM --platform=$BUILDPLATFORM golang:1.27.1-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195 AS build

ARG TARGETOS TARGETARCH
ARG VCS_REF=local-unverified

WORKDIR /src

ENV CGO_ENABLED=0 \
    GOWORK=/src/go.gateway.work

COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -mod=readonly -trimpath -o /out/grafana-ai-gateway ./ai-gateway/cmd/grafana-ai-gateway

RUN --mount=type=cache,target=/go/pkg/mod \
    set -eu; \
    mkdir -p /out/licenses/grafana-ai-gateway /out/licenses/dependencies; \
    cp ai-gateway/LICENSE ai-gateway/NOTICE /out/licenses/grafana-ai-gateway/; \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go list -deps -mod=readonly \
      -f '{{with .Module}}{{.Path}}{{"\t"}}{{if .Version}}{{.Version}}{{else}}local{{end}}{{"\t"}}{{if .Sum}}{{.Sum}}{{else}}-{{end}}{{"\t"}}{{.Dir}}{{end}}' \
      ./ai-gateway/cmd/grafana-ai-gateway | sort -u > /tmp/modules; \
    : > /out/licenses/THIRD_PARTY_MODULES.txt; \
    while IFS="$(printf '\t')" read -r module version checksum directory; do \
      if [ "$version" = local ]; then \
        case "$directory" in /src|/src/*) ;; *) echo "unexpected local module: $module" >&2; exit 1 ;; esac; \
        attribution="$VCS_REF"; \
      else \
        [ "$checksum" != - ] || { echo "missing module checksum: $module" >&2; exit 1; }; \
        attribution="$version"; \
      fi; \
      printf '%s\t%s\t%s\n' "$module" "$attribution" "$checksum" >> /out/licenses/THIRD_PARTY_MODULES.txt; \
      destination="/out/licenses/dependencies/${module}@${attribution}"; \
      mkdir -p "$destination"; \
      if [ "$version" = local ]; then \
        case "$directory" in /src/ai-gateway) license_dir=/src/ai-gateway ;; *) license_dir=/src ;; esac; \
      else \
        license_dir="$directory"; \
      fi; \
      for file in "$license_dir"/LICENSE* "$license_dir"/LICENCE* "$license_dir"/NOTICE*; do \
        [ -f "$file" ] || continue; \
        cp "$file" "$destination/"; \
      done; \
    done < /tmp/modules

FROM debian:bookworm-slim@sha256:88200866dfff7ea7f5cbcb6ec7c8a701889efe6fe859fe64d6990e4b07ea4171 AS runtime

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates \
  && rm -rf /var/lib/apt/lists/* \
  && groupadd --gid 10001 --system gateway \
  && useradd --uid 10001 --gid 10001 --system --no-create-home gateway

ARG VCS_REF=local-unverified
ARG SOURCE_URL=https://github.com/grafana/ai-sdk

LABEL org.opencontainers.image.title="Grafana AI Gateway" \
      org.opencontainers.image.description="Standalone Grafana AI Gateway" \
      org.opencontainers.image.source=$SOURCE_URL \
      org.opencontainers.image.revision=$VCS_REF \
      org.opencontainers.image.licenses="AGPL-3.0-only"

COPY --from=build /out/grafana-ai-gateway /usr/local/bin/grafana-ai-gateway
COPY --from=build /out/licenses /usr/share/licenses/grafana-ai-gateway

USER 10001:10001

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/grafana-ai-gateway"]
