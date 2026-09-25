# Run AI Gateway in a container

The repository builds a container for the `grafana-ai-gateway` command from
Gateway and local SDK/provider/middleware source in the checked-in
`go.gateway.work`. Gateway remains outside the root SDK-only `go.work`.
Published images use the source at the pushed revision, even if the internal
module pins in `ai-gateway/go.mod` are older. Gateway is released as an image,
not as a supported standalone Go module or importable application package.

After all required CI checks pass, pushes to `grafana/ai-sdk` publish Linux AMD64 and ARM64 images:

- Pushes to `main` publish `ghcr.io/grafana/ai-gateway:sha-<full-commit-sha>` for the pushed HEAD commit.
- Git tags matching `ai-gateway/v*` publish release images. For example, `ai-gateway/v0.1.0` publishes `ghcr.io/grafana/ai-gateway:v0.1.0`.

The workflow does not publish moving `main` or `latest` tags.

## Build the image

Run the build task from the repository root:

```bash
mise run build-ai-gateway-image
```

Local builds use the same Dockerfile, workspace and repository-root context as
CI, including uncommitted local source. Their OCI revision label and local-module
inventory say `local-unverified` rather than claiming the HEAD commit; published
images are built from a clean checkout and identify the push SHA. Set
`AI_GATEWAY_IMAGE` to build with another local image name:

```bash
AI_GATEWAY_IMAGE=example/ai-gateway:test mise run build-ai-gateway-image
```

## Configure models and secrets

Mount one model configuration file as read-only. Each provider uses
`apiKeyEnv` to name an environment variable. The YAML file contains the
reference, not the provider secret.

```yaml
providers:
  anthropic-primary:
    type: anthropic
    apiKeyEnv: ANTHROPIC_API_KEY
    baseURL: https://api.anthropic.com
models:
  grafana/assistant:
    name: Grafana Assistant
    primary:
      provider: anthropic-primary
      model: claude-sonnet-example
```

The command fails during startup if `ANTHROPIC_API_KEY` is unset or empty. Pass
the secret at runtime. Do not add it with a Docker build argument or image
environment instruction.

This example uses production authentication defaults:

```bash
docker run --rm \
  --read-only \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --stop-timeout 20 \
  --mount type=bind,source="$PWD/models.yaml",target=/etc/grafana-ai-gateway/models.yaml,readonly \
  --env GRAFANA_AI_GATEWAY_CONFIG_FILE=/etc/grafana-ai-gateway/models.yaml \
  --env GRAFANA_AI_GATEWAY_AUTH_JWKS_URL=https://identity.example.com/.well-known/jwks.json \
  --env ANTHROPIC_API_KEY \
  --publish 8080:8080 \
  ai-gateway:local
```

The image runs as UID and GID `10001`. Make the mounted configuration readable
by that identity.

Gateway process settings use the `GRAFANA_AI_GATEWAY_*` environment prefix.
Run the command with `--help` for the matching flags, defaults, and limits. The
container does not change the command's production defaults. This example
uses the default JWT-verifying mode; it is not the public Cloud deployment.
For client credentials and endpoint selection, see [Authenticate to Grafana AI
Gateway](gateway-authentication.md). For the trusted-proxy server mode, see the
[Cloud authentication contract](../../ai-gateway/docs/cloud-authentication.md).

## Use production endpoints

Production mode requires HTTPS for the JSON Web Key Set (JWKS) URL and every
custom provider base URL. Gateway startup rejects URLs with user information,
a query string, or a fragment. Configure the final endpoint because the
Gateway does not follow outbound redirects.

By default, the command listens on port 8080. The model discovery and
language-model routes require Gateway authentication. These operational routes
do not require authentication:

- `GET /live`
- `GET /ready`
- `GET /metrics`

Place the listener behind a trusted network boundary or apply network policy if
those routes must not be public.

The command's default graceful shutdown timeout is 15 seconds. Set the
container stop timeout to more than 15 seconds so the process can finish before
the runtime sends `SIGKILL`.

## Track source and dependencies

The image has Open Container Initiative (OCI) labels for the source repository,
source revision, and AGPL-3.0-only license. Publication sets the revision to
the full verified source commit. The Gateway-specific Docker context excludes
Git metadata, local configuration, credentials and development caches; supply
provider secrets only at runtime.

License files are under
`/usr/share/licenses/grafana-ai-gateway/`. The directory includes the Gateway
AGPL license and notice, the target-platform command's used modules (local
modules at the verified SHA or `local-unverified`, external modules at resolved
versions and checksums), inherited root Apache licensing for local SDK modules,
and discovered dependency license/notice files. SDK/provider/middleware Go
module releases still require separate standalone `GOWORK=off` validation;
passing image checks does not authorize their publication.

---

← [Testing model-backed code](testing.md) · [Docs index](../README.md) · [Production checklist →](../best-practices/production.md)
