# Local native-provider proxy

The public certificate in `cert.pem` and public test key embedded in
`native-provider-proxy.ts` let command integration tests reach local synthetic
adapters while the BYOK implementation retains its fixed native HTTPS
destinations. They are not provider credentials and must never be used outside tests.
The embedded key uses the scanner's inline `trufflehog:ignore` tag; adding that
comment inside a PEM file would break TLS parsing. The certificate has a test-only
preamble outside its PEM block.

Native-proxy cases use a command binary built with the `gatewaytest` tag. Its
bootstrap reads this certificate from `GATEWAY_TEST_CA_FILE` into an explicit
`x509.CertPool` and registers it with `x509.SetFallbackRoots`. The subprocess gets
`GODEBUG=x509usefallbackroots=1` and a local `HTTPS_PROXY`; this works with the
module's Go 1.26 minimum and newer Go versions on macOS without `SSL_CERT_FILE`
or system trust changes. Certificate and hostname verification remain enabled.

Ordinary command cases and production builds omit the tag and its trust bootstrap.
Negative command cases check missing/invalid test trust and production exclusion.
The proxy accepts only the explicitly supported test hostname. These responses are
focused integration fixtures, not recorded provider responses or parity evidence
about Vercel's hosted Gateway account-selection policy.

The self-signed certificate is valid from 2020 through 2040 and names
`api.anthropic.com` and `api.openai.com`. The proxy serves both native adapters;
request-only construction is also covered by Go transport tests.
