## 1. Core DoStream rewrite

- [x] 1.1 Implement first-chunk peek logic in `fallback.DoStream`: after `c.DoStream()` returns `(result, nil)`, read from the stream channel with `select` on both the channel and `ctx.Done()`
- [x] 1.2 Handle `PartError` first chunk: extract the error, drain the remaining stream in a goroutine, apply the decider, and continue the fallback loop
- [x] 1.3 Handle valid first chunk: create a wrapper goroutine + channel that replays the peeked chunk then forwards remaining chunks
- [x] 1.4 Handle empty stream (channel closed immediately): return a closed empty channel

## 2. Tests

- [x] 2.1 Add test: primary streams `PartError` as first chunk, secondary succeeds -- verify secondary stream is returned
- [x] 2.2 Add test: primary streams `PartError`, decider rejects -- verify error is returned without trying secondary
- [x] 2.3 Add test: all candidates stream `PartError` -- verify last error is returned
- [x] 2.4 Add test: primary succeeds with valid first chunk -- verify peeked chunk is replayed and all subsequent chunks arrive in order
- [x] 2.5 Add test: primary stream is empty (closed channel) -- verify empty stream returned without error
- [x] 2.6 Add test: stream error with context-length message -- verify default decider rejects fallback
- [x] 2.7 Add test: context cancelled during first-chunk read -- verify context error returned

## 3. Verification

- [x] 3.1 Run `make check` (fmt, vet, test) and verify all tests pass
