## Context

The registered baseline is `@ai-sdk/amazon-bedrock@5.0.55` and
`@ai-sdk/openai@4.0.41` at Vercel commit `d76eb85a`. Its Mantle subpath owns
routing/authentication and delegates model behavior to OpenAI internals. The Go
Bedrock module already implements Converse, but that request protocol is not
valid for the Mantle OpenAI-compatible endpoint.

The stacked prerequisite exposes `providers/openai.NewResponsesWithClient` and
separates model/response attribution from the OpenAI metadata namespace. The
official `openai-go/v3/bedrock` client already implements Bedrock bearer and
SigV4 authentication, credential refresh, retry re-signing, body hashing,
endpoint validation, environment isolation, and redirect protection.

## Goals / Non-Goals

**Goals:**

- Add the smallest honest Mantle Responses provider surface.
- Reuse official authentication and existing Responses conversion.
- Preserve bearer rollback and AWS credential-chain operation.
- Keep provider attribution and continuation metadata aligned with upstream.
- Support the AWS-documented OpenAI endpoint used by GPT-5.6 Luna.

**Non-Goals:**

- Implement or emulate Mantle Chat Completions.
- Reuse the parent Converse model or its request signing implementation.
- Duplicate SigV4, credential refresh, or retry logic.
- Claim a provenance-valid provider recording without a live Mantle capture.
- Change existing Bedrock Converse behavior.

## Decisions

### Add a nested Responses-only package

Create `providers/bedrock/mantle`, within the existing Bedrock Go module. Its
public constructor is:

```go
func NewResponses(
    ctx context.Context,
    modelID string,
    cfg Config,
    clientOpts ...option.RequestOption,
) (provider.LanguageModel, error)
```

`Config` and `TokenProvider` alias the official Bedrock client configuration so
callers receive the complete, maintained authentication surface without a
second configuration vocabulary. The constructor returns an error because the
official client validates Mantle configuration and may resolve AWS settings
before any inference call. Protected generic request-option overrides are
validated by the official request finalizer before transport.

A registry `Provider.LanguageModel` is intentionally omitted. Pinned upstream
maps its callable/default and `languageModel` surfaces to Chat, not Responses.
Presenting Responses as that default would be a parity bug.

### Delegate authentication to the official OpenAI Bedrock client

Construct `openai-go/v3/bedrock.NewClient`, then pass the resulting top-level
client to `providers/openai.NewResponsesWithClient`. This preserves the
security-sensitive finalizer ordering and signs each retry after method-level
headers and bodies are finalized. Copying options from a constructed client,
reusing the parent Converse signer, or writing a third signer were rejected
because they either duplicate authentication finalizers, couple incompatible
protocols, or duplicate security-sensitive logic.

The provider exposes both bearer and SigV4 modes through the official config.
Explicit ambiguous modes fail closed. This is a stricter Go adaptation than the
pinned TypeScript provider's bearer-first selection and avoids silently
ignoring configured AWS credentials.

### Separate attribution from continuation metadata

Construct the delegated model with provider identity
`bedrock-mantle.responses`. The OpenAI adapter continues to emit provider
options and metadata under `openai` (or its existing Azure fallback), matching
the pinned OpenAI internal model. This allows item IDs, encrypted reasoning,
tool namespaces, and approval correlation to round-trip through later prompts.

### Select the AWS route by model

The generic Mantle Responses surface uses
`https://bedrock-mantle.<region>.api.aws/v1`, matching the pinned TypeScript
provider and AWS's service-level documentation. GPT-5.6 Luna is a documented
exception: `openai.gpt-5.6-luna` is served at `/openai/v1/responses` on Mantle.
The constructor therefore resolves the region and supplies the appropriate
fully qualified default base to the official client before the delegated model
appends `/responses`.

Endpoint selection is the only behavior layered in front of the official
client; authentication and final request validation remain delegated. An
explicit `Config.BaseURL` or `AWS_BEDROCK_BASE_URL` is preserved so proxies and
future service-specific routes stay caller-controlled. Custom base URLs are
accepted only through the official Bedrock config, which prevents method-level
request options from overriding protected routing.

### Keep the stacked dependency explicit

Until the prerequisite PR merges, `providers/bedrock/go.mod` pins the resolvable
pseudo-version for PR #154's head. The follow-up PR targets #154's branch. After
#154 merges, retarget this PR to `main` and replace the temporary dependency pin
with the merged/released OpenAI provider revision before marking it ready.

### Use focused tests without invented provider fixtures

Unit tests use controlled HTTP transports to verify endpoint construction,
model ID preservation, bearer authorization, SigV4 scope/payload hashing,
provider identity, response attribution, and OpenAI metadata. They do not claim
service provenance. The parity map remains mixed/manual until a real Mantle
recording can be captured.

## Risks / Trade-offs

- **The stacked pseudo-version is temporary** → Keep the PR in draft and update
  the dependency after #154 merges.
- **Mantle paths vary by model** → Test both generic `/v1` routing and Luna's
  documented `/openai/v1` exception, and preserve explicit base URL overrides.
- **Chat remains unavailable** → Retain an explicit Mantle Chat/default-provider
  gap and do not add a registry default.
- **Official client configuration is part of this package's API** → Alias the
  pinned major-version types rather than copying fields that could drift.
- **No live service recording exists** → Limit claims to focused transport and
  delegation tests; require deployment validation before rollout.

## Migration Plan

1. Merge and publish the OpenAI provider prerequisite.
2. Retarget this stacked PR to `main` and update its OpenAI provider dependency.
3. Merge the Mantle Responses package with the Chat gap still recorded.
4. Update consumers to construct Mantle with dual-auth configuration, deploy
   IAM permission before selecting SigV4, and retain bearer rollback during the
   verification window.
