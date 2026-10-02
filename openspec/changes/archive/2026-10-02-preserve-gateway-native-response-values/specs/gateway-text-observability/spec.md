## ADDED Requirements

### Requirement: Operator capture is independent from returned native values

Gateway metadata-only logger, Prometheus, enrichment and Agent Observability observations SHALL retain their existing payload exclusions and canonical logical model identity when native warning/source/response identity values become caller-visible. Arbitrary warning strings, source ID/URL/title/filename and response ID/modelId SHALL NOT become operator attributes, metric labels or metadata-only exported payloads. The observer chain SHALL pass original results/parts unchanged to protocol encoding; operator capture restrictions SHALL NOT censor returned values. Known credentials and another tenant's state SHALL remain protected independently of ordinary application scalar/display values.

Consumer applications SHALL be able to configure existing reusable middleware independently around providers/grafana without enabling server capture. Tests SHALL use middleware.WrapLanguageModel with Middleware.WrapGenerate to inspect actual unary warnings/content/Response.Body and WrapStream to observe forwarded PartStreamStart/PartSource/PartResponseMeta through a context-aware test tee. These hooks SHALL receive contracted values without mutating output; hook access SHALL NOT be equated with automatic built-in capture/export.

A separately configured consumer logger.Middleware with CaptureOptions.ResponseBody and a consumer-owned slog destination SHALL prove actual opt-in logging of the bounded Gateway unary body, including native nested identity/warnings/sources within its configured capture budget. Raw-body accessibility alone SHALL NOT be called capture proof. Tests/docs SHALL distinguish that logged Gateway body from native transport diagnostics and from typed native Response identity overwritten by both clients. This capability SHALL NOT add middleware/API surface, universal stream-warning/source export, an unsupported Agent Observability source recording representation or a diagnostic carrier.

#### Scenario: Native identity differs from canonical route
- **WHEN** an alias resolves a canonical route whose provider supplies a different native response ID/modelId
- **THEN** returned streaming identity and raw unary response identity SHALL retain native values
- **AND** logical operator metrics/logs/exports SHALL retain canonical identity and exclude native payload values

#### Scenario: Native warnings and display are returned without server capture
- **WHEN** a metadata-only configured service returns native warnings and URL/document sources with native IDs and file-path display
- **THEN** callers SHALL receive those values unchanged within protocol bounds
- **AND** server logger/metrics/metadata-only Agent Observability exports SHALL omit their arbitrary strings

#### Scenario: Consumer capture is separately configured
- **WHEN** consumer-owned middleware wraps providers/grafana with independent capture settings/destinations while server capture remains metadata-only
- **THEN** WrapGenerate/WrapStream tests SHALL observe contracted fields and a separate opt-in consumer logger test SHALL assert actual Gateway response-body capture at its own destination
- **AND** tests SHALL prove typed unary identity remains replaced while native identity is present in the Gateway response body
- **AND** neither observation location SHALL mutate responses to satisfy the other's capture settings
- **AND** hook access SHALL NOT imply automatic source/warning capture by every built-in middleware
