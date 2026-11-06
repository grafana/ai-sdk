# Durable stored completion backend

Use the existing app-sdk resource client and generated kind. Explicit protocol-only
roles prevent automatic generated reader/editor roles from exposing conversations.
The adapter supplies a verified owner and sanitized public record; the resource
backend enforces ownership, expiry and optimistic concurrency independently.
