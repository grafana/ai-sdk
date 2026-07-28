## 1. Capability and Request Conversion

- [x] 1.1 Add Bedrock-local adaptive-thinking capability detection for the registered upstream model families.
- [x] 1.2 Derive adaptive thinking and mapped effort from root reasoning for capable Anthropic models while preserving the budget path for older models.
- [x] 1.3 Merge explicit provider reasoning fields over derived values and complete older-model maximum-token capabilities.
- [x] 1.4 Preserve merged type and budget fields in Nova-style reasoning config serialization.
- [x] 1.5 Gate Nova-style budget serialization to enabled thinking while retaining raw-option warnings.

## 2. Regression Coverage

- [x] 2.1 Add unit coverage for adaptive capability detection and both root-reasoning request branches.
- [x] 2.2 Add an upstream-generated Bedrock request conformance fixture for adaptive root reasoning.
- [x] 2.3 Add coverage for partial provider config merging, disabled cleanup, and older Sonnet/Opus budgets.
- [x] 2.4 Add coverage for Nova-style merged reasoning config serialization and warnings.
- [x] 2.5 Add coverage that adaptive Nova reasoning omits the unsupported nested budget.

## 3. Validation

- [x] 3.1 Run Bedrock provider tests and vet.
- [x] 3.2 Run strict OpenSpec validation and the parity check.
