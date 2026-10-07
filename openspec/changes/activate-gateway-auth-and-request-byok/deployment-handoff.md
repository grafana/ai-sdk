# Deployment-tools handoff

Status: prepared in this repository only. No deployment-tools or backend-enterprise
files have been changed. No environment activation or deployed isolation proof is
claimed. Task 4.1 remains separately authorized work and needs a delivery owner/link.

## Required delivery

- Replace the old mode/listener settings with private API `:8080`, Cloud API `:8081`
  and operational `:8082` addresses, or explicit deployment-chosen equivalents.
  Use the command's `GRAFANA_AI_GATEWAY_SERVER_PRIVATE_LISTEN_ADDRESS`,
  `GRAFANA_AI_GATEWAY_SERVER_CLOUD_LISTEN_ADDRESS` and
  `GRAFANA_AI_GATEWAY_SERVER_OPERATIONAL_LISTEN_ADDRESS` settings.
- Provide separate Service ports/endpoints for approved internal JWT clients,
  the existing authenticating Cloud edge, and probes/monitoring. Names remain a
  deployment-tools decision; none are assumed to exist.
- Configure regional `GRAFANA_AI_GATEWAY_AUTH_JWKS_URL` trust and audience `ai-sdk`.
  Keep the configured model file and its provider-secret environment references
  available for private configured-account calls only. Never enable unsafe JWT
  verification in an environment.
- Keep JWT ingress private. Do not add a public JWT route, k6-specific allowlist,
  backend-enterprise JWT verifier, or alternate authentication fallback.
- Preserve backend-enterprise's existing Cloud CAP scope/realm/stack/IP checks,
  replacement of `X-Scope-OrgID`, and removal of `Authorization`, `X-Access-Token`
  and `X-Grafana-Id` before the Cloud application listener.
- Restrict Cloud application ingress to the authenticating edge's workload identity
  and namespace. Explicitly deny direct access by internal JWT workloads. A
  shared namespace allow rule must not accidentally authorize the Cloud port.
- Restrict private API ingress to approved internal clients, and operational ingress
  to monitoring/probes. Health and metrics targets move to the operational port.
- Preserve bounded egress for configured providers, fixed native Anthropic/OpenAI
  HTTPS endpoints, regional JWKS, and configured observability export. Increasing
  termination grace does not defer cancel-first shutdown of active calls.

## Acceptance evidence before activation

Rendered-manifest checks must prove the three target ports, no public JWT ingress,
proxy-only Cloud application access and explicit private/operational rules.
Then collect deployed positive and negative connection results:

1. Approved internal workload + valid private JWT can discover/invoke configured
   models; missing/invalid JWT and forged Cloud assertions fail.
2. Public Cloud requests traverse the authenticating edge. Valid scoped CAP + BYOK
   can invoke an explicit native model; discovery is the documented unsupported
   error. Invalid CAP/scope/realm/IP requests stop at the edge.
3. An internal JWT workload cannot establish a connection directly to the Cloud
   application port, even when supplying a syntactically valid `X-Scope-OrgID`.
4. The private JWT API cannot be reached through public ingress; probes and metrics
   are reachable only through their approved operational path.
5. Both populations cancel on rollout/shutdown and every listener becomes unready.

Attach the environment, rendered revision, workload identities, exact probes and
results to the separately owned delivery. Local dummy-edge/TLS fixtures and the
Go catalog spies do not replace this proof.

## Prepared issue/plan update

The basic #317 delivery supports Go and pinned Vercel request-only API-key BYOK for
native Anthropic and OpenAI Responses, with configured access isolated behind the
private JWT API. Ordered credentials reuse `fallback.New` default semantics;
non-retryable 401/403 stop, retryable/unknown pre-commit failures may advance, and
any first stream part commits. Duplicate provider-side work remains possible.

Do not close all of #317 on this basis. Alternate credential families, extended
routing, private per-credential attempt evidence and live/deployed acceptance
remain outside this basic delivery. Shared native-option work stays coordinated
with #318; neighboring #318–#324 and #280 retain their separate scope. This text is
a prepared update, not an external issue comment or an edit to the external plan.
