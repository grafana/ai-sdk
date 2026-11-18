# Tasks

- [x] 1. Add a test for an error part then cancellation, a denial then cancellation, a deadline, a plain cancellation, and cancellation before an error part or denial, plus a direct test that error parts observed after cancellation are ignored, and confirm each fix's removal fails its cases.
- [x] 2. Rank an observed error or denial over cancellation in `finalizeStream`, and ignore error parts received after cancellation.
- [x] 3. Document the precedence in the package documentation and the spec.
