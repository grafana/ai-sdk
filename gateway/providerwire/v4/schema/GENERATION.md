# ProviderWire V4 generator evaluation

Production generation is deferred. This evaluation uses a standalone corpus and
does not generate from the normative request, result, stream, or error schemas.
No generated Go file is committed.

## Inputs

- `evaluation/difficult-unions.json`
- `evaluation/difficult-unions-openapi.yaml`

The equivalent inputs cover message, tool, and file unions; inactive siblings;
optional false; absent versus explicit empty collections; nullable and non-null
opaque JSON; object-valued keyed maps; and raw union JSON.

## Candidates

| Candidate | Version | Invocation mode |
| --- | --- | --- |
| `github.com/atombender/go-jsonschema` | `v0.24.1` | JSON Schema 2020-12 input |
| `github.com/oapi-codegen/oapi-codegen/v2` | `v2.8.0` | OpenAPI 3.1 input, `types,skip-prune` |

The oapi-codegen compile check uses `github.com/oapi-codegen/runtime@v1.7.0`,
which is required by the generated union helpers. Without `skip-prune`, the
empty-path evaluation document produces no model types.

## Reproduction

Run from the repository root. The script pins both generators, creates two clean
outputs for each, compares them byte-for-byte, compiles each candidate in an
isolated module, executes candidate-specific semantic round-trip tests, and
removes all output on exit.

```sh
bash gateway/providerwire/v4/schema/evaluation/evaluate.sh
```

The semantic tests exercise optional false, explicit empty collections,
inactive union siblings, nullable and non-null opaque JSON, keyed object maps,
and raw/semantic union preservation. The evaluation was run with the
repository's Go 1.26 toolchain. Both candidates produced byte-identical output
on two clean runs and passed the tests that assert their observed passes and
losses.

## Results

| Gate | go-jsonschema 0.24.1 | oapi-codegen 2.8.0 |
| --- | --- | --- |
| Clean deterministic generation | Pass | Pass with `skip-prune` |
| Generated code compiles | Pass | Pass with runtime dependency |
| Optional false presence | Pass: pointer boolean | Pass: pointer boolean |
| Explicit empty versus absent array | Fail: `omitempty` collapses them | Pass: pointer collection preserves explicit empty |
| Exact selected union arm | Fail: generated union is `interface{}` | Fail: raw union helpers do not validate exactly one arm or reject inactive siblings |
| Nullable versus non-null opaque JSON | Fail: both become `interface{}` | Fail: both become `interface{}` |
| Object-valued keyed map | Pass | Pass |
| Raw union JSON preservation | Semantic interface value only | Pass through `json.RawMessage` union storage |
| Required-field validation on decode | Partial root checks only | Fail without a separate validator |
| Manual repair required for contract | Yes | Yes |

The JSON-Schema-native candidate emits `interface{}` for the message, tool, and
file unions. The OpenAPI candidate emits raw union wrappers and useful typed
accessors, but decoding does not establish discriminator exclusivity or closed
selected-arm validation. The JSON-Schema candidate loses explicit empty
collections; the OpenAPI candidate preserves them with a pointer collection.
Both accept null for the non-null opaque field.

## Decision

Neither candidate passes every gate without manual edits, so H1 adopts neither.
A future runtime may use generated code only behind separate strict validation
and presence-aware adaptation after a new evaluation. H1 keeps curated schemas
as the authority and contains no generated production DTOs.
