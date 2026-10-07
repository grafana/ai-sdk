# Local native-provider proxy

These public, test-only TLS credentials let command integration tests reach local
synthetic adapters while the BYOK implementation retains its fixed native HTTPS
destinations. They are not provider credentials and must never be used outside tests.

The test subprocess receives this certificate through `SSL_CERT_FILE` and a local
`HTTPS_PROXY`; neither the system trust store nor production configuration changes.
The proxy accepts only the explicitly supported test hostname. These responses are
focused integration fixtures, not recorded provider responses or parity evidence
about Vercel's hosted Gateway account-selection policy.

The self-signed certificate is valid from 2020 through 2040 and names
`api.anthropic.com` and `api.openai.com`. The proxy serves both native adapters;
request-only construction is also covered by Go transport tests.
