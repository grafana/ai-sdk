## MODIFIED Requirements

### Requirement: Continuation metadata replacement
The service SHALL preserve supported reasoning providerMetadata through #280's ordinary bounded object-valued namespace contract rather than only a closed continuation-key table. Unknown namespace/key values and nested null, empty string, object and array distinctions SHALL remain at their original positions. Absent versus explicit empty metadata SHALL remain distinguishable where represented; top-level/namespace null SHALL not be claimed as registered object-valued metadata. Core assembly SHALL retain latest non-nullish object replacement, not recursive merge. Request authorization/protected fields and selected-backend option policies SHALL remain effective. Continuation data SHALL never enter metadata-only telemetry.

#### Scenario: Signature arrives on end
- **WHEN** an end event carries a final signature or encrypted content
- **THEN** the next assistant history and actual subsequent native provider request SHALL retain that final value

#### Scenario: Future metadata replaces previous object
- **WHEN** later reasoning metadata supplies a future namespace and nested null values
- **THEN** #280 evidence SHALL prove the latest object's supported values survive through both clients and subsequent requests without deep-merging older keys
