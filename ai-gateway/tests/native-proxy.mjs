// End-to-end stand-in for the native provider endpoints. Stack catalogs have
// no endpoint setting, so their calls go to api.anthropic.com; Grafana's
// plugin reaches this proxy through HTTPS_PROXY and trusts the certificate
// scripts/e2e.sh generates for each run. It reuses the ProviderWire suite's
// proxy and forwards Anthropic tunnels to the fake provider.
import { readFileSync } from 'node:fs';
import { startNativeProviderProxy } from './native-provider-proxy.ts';

await startNativeProviderProxy(
  { 'api.anthropic.com:443': 'http://fake-anthropic:8080' },
  {
    cert: readFileSync('/etc/native-proxy/cert.pem'),
    key: readFileSync('/etc/native-proxy/key.pem'),
    host: '0.0.0.0',
    port: 8443,
  }
);
