## 1. Regression coverage

- [x] 1.1 Reproduce the invalid-output failure for interrupted finishes with open text, reasoning and tool input.
- [x] 1.2 Cover other reasons, pending previews (including history), segment boundaries and abandoned tool IDs.

## 2. Runtime and consumers

- [x] 2.1 Implement the shared interrupted transition without synthesizing output.
- [x] 2.2 Verify real Gateway output with both clients, extension observations and the logical observation chain.
- [x] 2.3 Verify native adapter mapping remains strict; update Gateway architecture and validate the module, interop and parity checks.
