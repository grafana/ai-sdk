# syntax=docker/dockerfile:1.27@sha256:bde3983e9c939224420ddaf6b784cc30e09b035a4dea01f581230c50809f372e

FROM --platform=$BUILDPLATFORM golang:1.26.6-bookworm@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36 AS build

ARG TARGETOS TARGETARCH

WORKDIR /src

ENV CGO_ENABLED=0 \
    GOWORK=off

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY LICENSE NOTICE ./
COPY catalog ./catalog
COPY providerwire ./providerwire
COPY cmd ./cmd

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -mod=readonly -trimpath -o /out/grafana-ai-gateway ./cmd/grafana-ai-gateway

RUN --mount=type=cache,target=/go/pkg/mod \
    set -eu; \
    mkdir -p /out/licenses/grafana-ai-gateway /out/licenses/dependencies; \
    cp LICENSE NOTICE /out/licenses/grafana-ai-gateway/; \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go list -deps -mod=readonly \
      -f '{{with .Module}}{{if .Version}}{{.Path}}{{"\t"}}{{.Version}}{{"\t"}}{{.Dir}}{{end}}{{end}}' \
      ./cmd/grafana-ai-gateway | sort -u > /out/licenses/THIRD_PARTY_MODULES.txt; \
    while IFS="$(printf '\t')" read -r module version directory; do \
      destination="/out/licenses/dependencies/${module}@${version}"; \
      mkdir -p "$destination"; \
      for file in "$directory"/LICENSE* "$directory"/LICENCE* "$directory"/NOTICE*; do \
        [ -f "$file" ] || continue; \
        cp "$file" "$destination/"; \
      done; \
    done < /out/licenses/THIRD_PARTY_MODULES.txt

FROM debian:bookworm-slim@sha256:88200866dfff7ea7f5cbcb6ec7c8a701889efe6fe859fe64d6990e4b07ea4171 AS runtime

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates \
  && rm -rf /var/lib/apt/lists/* \
  && groupadd --gid 10001 --system gateway \
  && useradd --uid 10001 --gid 10001 --system --no-create-home gateway

ARG VCS_REF=unknown
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
