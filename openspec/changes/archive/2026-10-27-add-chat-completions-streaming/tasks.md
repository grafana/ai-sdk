## 1. Protocol prototype

- [x] 1.1 Enable streaming dispatch without changing unary request behavior.
- [x] 1.2 Encode text/function chunks, finish, optional usage and success marker.
- [x] 1.3 Preserve early native identity and expose warnings/late metadata.
- [x] 1.4 Add runtime streaming/cancellation/cleanup integration without duplicate size limits.

## 2. Evidence and scope

- [x] 2.1 Record the remaining #483 blocker after #555/#579/#581 integration.
- [x] 2.2 Keep route/authentication and TypeScript command proof out of scope.
- [x] 2.3 Run package tests including official OpenAI Go and cleanup scenarios.
- [x] 2.4 Validate the change, sync its capability and archive it with evidence.

## 2026-10-28 architecture revision

- [x] Delegate setup/waits/cleanup to Call.Stream, Stream.Next and Stream.Close.
- [x] Remove private setup workers, idle timers and drain loops.
- [x] Reject additional native segments after first finish rather than truncate.
- [x] Record native-activity idle semantics and the pending #483 validator.

#555/#579/#581 provide execution/visibility/host contracts. #483 remains a
mandatory readiness dependency. Fresh integrated evidence belongs to the
route/client change; historical checked tasks are not proof of that gate.
