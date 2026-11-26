## 1. Final step accessor

- [x] 1.1 Return the last recorded step by value for both result types, preserving existing drain and error behavior.

## 2. Evidence

- [x] 2.1 Exercise multi-step StreamText, GenerateText and Agent.Generate with synthetic result data.
- [x] 2.2 Exercise zero steps, incomplete output, failure and cancellation, and verify the value-copy boundary and concurrent reads under the race detector.
- [x] 2.3 Run the owning module tests and lint.
- [x] 2.4 Verify the registered parity suite on the candidate head in CI.
