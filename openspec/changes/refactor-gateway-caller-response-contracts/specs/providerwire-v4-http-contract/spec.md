## MODIFIED Requirements

### Requirement: Focused unary client-consumption evidence
Exact registered injected-response probes SHALL assert supported content/finish/usage, delivered warning preservation/combination and Gateway response headers. Local request/response SHALL overwrite server-native typed transport, with server warnings preceding client warnings. Supplied registered response id/modelId/timestamp SHALL remain in bounded raw response.body, not typed unary response. Current metadata behavior SHALL remain unchanged until #280; no typed identity or future metadata parity SHALL be claimed.

#### Scenario: Transport overwrite does not erase warnings
- **WHEN** the body contains actual identity and ordered server warnings
- **THEN** typed transport SHALL be client-owned, warnings SHALL survive before client warnings and native identity SHALL remain raw-body-only

### Requirement: Focused streaming client-consumption evidence
Exact registered SSE probes SHALL retain clean EOF/DONE/raw filtering/timestamp behavior and prove delivered warning/source ID/display/actual identity semantics. Current metadata transport SHALL remain unchanged; #280 ordinary metadata/source integration/continuation is later owner acceptance, not a foundation gate. No additional stream-lifecycle layer SHALL be introduced to deliver diagnostics.

#### Scenario: Foundation stream is consumed
- **WHEN** valid current parts carry warnings/native source display/actual identity and finish at clean EOF
- **THEN** registered values/order/timestamp semantics SHALL survive without claiming unimplemented metadata placements

### Requirement: Focused non-success client-consumption evidence
Exact registered HTTP probes SHALL prove minimal direct-provider versus fixed Gateway error category/message/status/retry and bounded native type/code/detail access through existing param/cause/body. Existing Go string Code/error API SHALL remain; no new typed detail properties or full native top-level-code equivalence is implied. HTTP 408/409/429/5xx retry and provider override gaps SHALL be explicitly tested; plain forwarded SSE error values remain distinct from HTTP error classes. Strict server output/bounds/security are independent of permissive client acceptance.

#### Scenario: Minimal error diagnostics survive
- **WHEN** a reviewed direct non-2xx response contains native status and bounded detail in registered param
- **THEN** both clients SHALL expose contracted existing fields and cause/body detail with pinned retry behavior

#### Scenario: Client parser is not server oracle
- **WHEN** the pinned client accepts additional malformed or arbitrary data
- **THEN** its normalization SHALL not define approved server output/security/bounds
