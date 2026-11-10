// End-to-end stand-in for the native provider endpoints. Stack catalogs have
// no endpoint setting, so their calls go to api.anthropic.com; Grafana's
// plugin reaches this proxy through HTTPS_PROXY and trusts the certificate
// scripts/e2e.sh generates for each run. It reuses the ProviderWire suite's
// proxy and forwards Anthropic tunnels to the fake provider.
import { readFileSync } from 'node:fs';
import { startNativeProviderProxy } from './native-provider-proxy.ts';

const certificates = process.env.NATIVE_PROXY_CERTIFICATES ?? '/etc/native-proxy';

await startNativeProviderProxy(
  { 'api.anthropic.com:443': process.env.NATIVE_ANTHROPIC_TARGET ?? 'http://fake-anthropic:8080' },
  {
    cert: readFileSync(`${certificates}/cert.pem`),
    key: readFileSync(`${certificates}/key.pem`),
    host: '0.0.0.0',
    port: Number(process.env.PORT ?? 8443),
  }
);
