## Decisions

- Share the provider minimum with operator validation. Only the operator requires models and `apiKeyEnv`. The stack resource still requires the models map, which may be empty. An undecodable parent supplies no providers or keys and fails the shared check.
- Reuse the provider fields in the new catalog form and in the connect dialog. Send the key only in the secure create member. The Gateway still decides whether an endpoint is allowed.
- Pass a one-shot health marker in navigation state. Consume it by replacing the history entry, and check only after the saved catalog is loaded and if the caller can read and check it. Use the same progress, error, and result handling as a manual check.
- Keep the fallback guard for removing a provider, and disable removal when only one remains. The last model remains removable.
