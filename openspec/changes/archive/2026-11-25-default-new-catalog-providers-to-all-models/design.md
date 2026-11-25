## Decision

Have the shared provider form disclose and submit `serveAllModels: true` for the new provider only. Both create and connect already use that form, so they need no separate policy or write. Writers can disable unlisted serving on the saved provider card. Direct API writes that omit the field still serve only listed models.

This is Grafana catalog UI policy, not an upstream `@ai-sdk/gateway` or ProviderWire change.
