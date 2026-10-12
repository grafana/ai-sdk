# Mid-output Anthropic fallback

`input.chunks.txt` is copied unchanged from
[`anthropic-fallback-mid-output.chunks.txt`](https://github.com/vercel/ai/blob/45e1fbc3ea10d22f852e12b1c1851e289c85e835/packages/anthropic/src/__fixtures__/anthropic-fallback-mid-output.chunks.txt)
in the registered Anthropic package. This is imported upstream fixture evidence,
not a live provider recording. `INDEX.yaml` records the import; inventory checks
verify byte identity against the registered source.

The replay preserves the fallback marker between separate signed reasoning blocks.
Focused provider tests additionally exercise outbound continuation and invalid
fallback metadata; frontend tests exercise schema-parsed SSE and model conversion.
The fixture alone does not prove live provider acceptance or default model-family
request capabilities.
