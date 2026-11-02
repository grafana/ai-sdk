## Context

The registered baseline is `@ai-sdk/provider-utils@5.0.68`, upstream commit
`612be2311464804d1e4897afacbf8f3fc2f6d9e3`. Its
[validator](https://github.com/vercel/ai/blob/612be2311464804d1e4897afacbf8f3fc2f6d9e3/packages/provider-utils/src/validate-download-url.ts)
and [tests](https://github.com/vercel/ai/blob/612be2311464804d1e4897afacbf8f3fc2f6d9e3/packages/provider-utils/src/validate-download-url.test.ts)
reject `.internal` after lowercasing and stripping trailing dots, while allowing
names such as `internal.example.com` and `service.notinternal`.

## Goals / Non-Goals

Match that hostname rule for generated-file downloads. Leave prompt downloading,
DNS pinning, private-address checks, redirect limits and upstream pins unchanged.

## Decisions

Extend `validateGeneratedFileURL` with one suffix check. Both the initial URL and
the client's redirect callback already use it, so no new configuration or helper
is needed. This corrects an implementation bug against the registered reference.

Focused tests inject a resolver that returns a public address and a dial function.
They verify initial rejection before either callback and redirect rejection before
either callback for the redirect target. Local HTTP fixtures test redirects;
these are synthetic tests, not provider recordings. Existing public/private IP,
DNS fallback, cancellation and successful redirect tests remain the regression
contract. The coverage map's mixed core orchestration surface is unchanged;
there is no provider or UI boundary change requiring new recorded fixtures.

## Risks / Trade-offs

Previously accepted generated-file URLs under `.internal` now fail even with
public DNS answers. This is the intended upstream behavior. Allowed lookalikes
and `.internal` in paths remain accepted by hostname validation.
