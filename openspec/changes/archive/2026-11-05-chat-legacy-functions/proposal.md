# Legacy Chat function compatibility

Support deprecated OpenAI functions/function_call request and response fields, including function-role continuations, by normalizing to the common tool model. Do not change native provider builders or weaken tool validation. Generated internal call IDs connect legacy history without exposing invented IDs to legacy clients. Reject ambiguous modern/legacy controls and parallel legacy calls.

The OpenAI Chat create reference documents these deprecated fields. The pinned upstream Vercel provider contract (4.0.35 at 028a025398782f4938303f48fbeee45c79af544b) uses common tool-call/result IDs; normalization is a Gateway adaptation. Focused synthetic unit tests and official Go/TypeScript plugin tests cover the boundary, without claiming live-provider recordings.
