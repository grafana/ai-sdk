import { createGateway } from "@ai-sdk/gateway";

export interface ServiceAccountGatewayOptions {
  grafanaURL: string;
  namespace: string;
  token: string;
  fetch?: typeof globalThis.fetch;
}

export function serviceAccountGateway({ grafanaURL, namespace, token, fetch }: ServiceAccountGatewayOptions) {
  const base = grafanaURL.replace(/\/+$/, "");
  return createGateway({
    apiKey: token,
    baseURL: `${base}/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/${encodeURIComponent(namespace)}`,
    fetch,
  });
}
