## 1. Request evidence

- [x] 1.1 Add synthetic OpenAI unary and streaming request tests for placement, absence, namespaces, grouped wrappers, excluded result paths, and invalid declarations before HTTP.

## 2. Result history

- [x] 2.1 Add the typed result option and its raw namespace and definition validation.
- [x] 2.2 Emit additional tools next to ordinary and grouped function results without affecting other result paths.

## 3. Verification

- [x] 3.1 Verify GenerateText and StreamText forward supplied result options to a second native request without additional execution.
- [x] 3.2 Run provider and core tests, targeted race tests and parity checks; review against the registered source and synchronize the spec.
