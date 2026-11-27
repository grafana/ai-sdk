#!/usr/bin/env bash
set -euo pipefail

output=${1:?give an invocation-owned output directory}
[[ "$output" = /* && -d "$output" ]]
cd "$(dirname "${BASH_SOURCE[0]}")/.."
export GOWORK=off
go build -race -mod=readonly -o "$output/gateway" ./pkg
go build -race -mod=readonly -o "$output/plugin-host" ./test/platformhost/cmd/plugin-host
