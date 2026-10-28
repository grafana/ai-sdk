# Spec Delta

## MODIFIED Requirements

### Requirement: Direct candidate references exclude recursive routes
A model route SHALL contain one required direct `primary` reference and zero or more ordered direct `fallback` references; each reference SHALL contain exactly a named provider instance and opaque backend model ID, plus an optional `serverFallbacks` policy owned by gateway-anthropic-server-fallbacks that is not a candidate reference. References to public routes and nested fallback definitions SHALL not be representable, making self-reference and recursive route graphs invalid under strict decoding.

#### Scenario: Direct candidate references exclude recursive routes
- **WHEN** a route attempts to reference a public route or nest a fallback inside a candidate
- **THEN** strict decoding SHALL reject it rather than construct a recursive route graph

#### Scenario: Policy does not create a distinct candidate
- **WHEN** a route's fallback repeats the primary's provider/backend tuple with a different `serverFallbacks` policy
- **THEN** startup SHALL fail with the existing repeated-candidate diagnostic, because the policy names server-side fallback models rather than public routes or candidates
