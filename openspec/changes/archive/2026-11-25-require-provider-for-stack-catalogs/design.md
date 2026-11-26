## Decisions

- Share the provider minimum with operator validation. Only the operator requires models and `apiKeyEnv`. The stack resource still requires the models map, which may be empty. An undecodable parent supplies no providers or keys and fails the shared check.
- Reuse the provider fields in the new catalog form and in the connect dialog. Send the key only in the secure create member. Clear the entered key if its type or endpoint changes. The Gateway still decides whether an endpoint is allowed.
- Accept the health marker only on a fresh push from creation and immediately replace the history entry without it. Keep the pending check in memory until that view loads the saved catalog. A refresh or history traversal never starts this check. Use the same progress, error, and result handling as a manual check.
- Keep the fallback guard for removing a provider, and disable removal when only one remains. The last model remains removable.
