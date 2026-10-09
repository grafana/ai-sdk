# Run AI Gateway in a container

Published `grafana-ai-gateway` images build Gateway and SDK source from the
same repository revision. Gateway is distributed as a container image.

After all required CI checks pass, pushes to `grafana/ai-sdk` publish Linux AMD64 and ARM64 images:

- Pushes to `main` publish `ghcr.io/grafana/ai-gateway:sha-<full-commit-sha>` for the pushed HEAD commit.
- Git tags matching `ai-gateway/v*` publish release images. For example, `ai-gateway/v0.1.0` publishes `ghcr.io/grafana/ai-gateway:v0.1.0`.

The workflow does not publish moving `main` or `latest` tags.

## Build the image

Run the build task from the repository root:

```bash
mise run build-ai-gateway-image
```

Local builds include uncommitted source and are labeled `local-unverified`.
Set `AI_GATEWAY_IMAGE` to build with another local image name:

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
  --publish 127.0.0.1:8082:8082 \
  ai-gateway:local
```

The image runs as UID and GID `10001`. Make the mounted configuration readable
by that identity.

Gateway process settings use the `GRAFANA_AI_GATEWAY_*` environment prefix.
Run the command with `--help` for the matching flags, defaults, and limits. The
container does not change the command's production defaults. This example
publishes only the private JWT API on the host's loopback interface; it does not
publish the Cloud application or operational listeners.
For client credentials and endpoint selection, see [Authenticate to Grafana AI
Gateway](gateway-authentication.md). For listener isolation and the trusted-proxy boundary, see the
[Cloud authentication contract](../../ai-gateway/docs/cloud-authentication.md).

## Use production endpoints

Production mode requires HTTPS for the JSON Web Key Set (JWKS) URL and every
custom provider base URL. Gateway startup rejects URLs with user information,
a query string, or a fragment. Configure the final endpoint because the
Gateway does not follow outbound redirects.

By default, trusted-Cloud BYOK requests use port 8080, operational traffic uses
port 8081, and private JWT requests use port 8082. API routes require their
listener's authentication policy. Only port 8081 serves these unauthenticated
operational routes:

- `GET /live`
- `GET /ready`
- `GET /metrics`

Keep private JWT access private, permit only the authenticating Cloud edge on
port 8080, and restrict port 8081 to monitoring and probes. Do not activate the
Cloud path before deployed network-isolation checks pass. Configured model
secrets in this example are available only through the private API; Cloud callers
supply request-only provider credentials.

The command's default graceful shutdown timeout is 15 seconds. Set the
container stop timeout to more than 15 seconds so the process can finish before
the runtime sends `SIGKILL`.

## Track source and dependencies

The image labels identify its source repository, revision (the pushed commit
for published images), and AGPL-3.0-only license. Licenses and the module
inventory are under `/usr/share/licenses/grafana-ai-gateway/`, including
Gateway and SDK licenses, dependency notices, and resolved module versions
and checksums. When distributing the image, include the approved
corresponding-source offer for that revision and its dependencies.

---

← [Testing model-backed code](testing.md) · [Docs index](../README.md) · [Production checklist →](../best-practices/production.md)
