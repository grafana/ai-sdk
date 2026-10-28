## 1. Contract and mapping

- [x] 1.1 Isolate the AGPL unary package and closed request schema from PR #248.
- [x] 1.2 Map supported text, function continuation, structured output and
  reasoning settings, preserving response-format name presence.
- [x] 1.3 Reject streaming before resolution and keep production routing and
  authentication outside this change.
- [x] 1.4 Add per-call native translation through the shared host and no-op behavior for other protocols.

## 2. Responses and ownership

- [x] 2.1 Map unary identity, usage, warnings and validated function/JSON output.
- [x] 2.2 Use fixed safe errors and inherit host context without adding the old
  request/response byte caps or model duration.
- [x] 2.3 Compare native defaults with registered upstream and record remaining
  resource and evidence limits in the design.

## 3. Verification and archive

- [x] 3.1 Run the unary package tests, including official OpenAI Go consumption.
- [x] 3.2 Validate this OpenSpec change with the repository-pinned CLI.
- [x] 3.3 Sync the capability specification and archive the completed change.

## 2026-10-28 architecture revision

- [x] Replace the resolver/private unary worker with Runtime.Call/Call.Generate.
- [x] Supply execution.WithTranslator; leave candidate wrapping to the host.
- [x] Map runtime categories and implement host.ErrorWriter without classifying
  raw native errors in Chat.

Fresh integration verification is tracked by the route/client change. The
shared #483 validator is still pending and is not checked off by this archive.
