# Protect request-account keys in native summaries

Change the earlier policy that intentionally preserved caller-owned BYOK key echoes. Validated request-account keys now join configured keys in the execution runtime's per-call summary redactor. Native summaries retain useful message, type, code and status without echoing account keys.

The selector obtains keys only from validated decoded destinations. No adapter reparses credentials, no runtime-global policy is mutated, and source errors, opaque metadata, content and operator capture remain unchanged. This is an intentional Gateway diagnostic-policy change; it does not alter upstream provider conversion or the pinned Vercel wire contract.
