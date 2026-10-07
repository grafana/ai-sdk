const MAX_CAPTURE_BYTES = 65_536;
const OMITTED = { capture: "omitted" } as const;

function object(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" &&
    (Object.getPrototypeOf(value) === Object.prototype || Object.getPrototypeOf(value) === null);
}

export function redactGatewayRequestForCapture(body: unknown): unknown {
  try {
    if (typeof body === "string") {
      if (new TextEncoder().encode(body).length > MAX_CAPTURE_BYTES) return OMITTED;
      body = JSON.parse(body);
    }
    if (!object(body)) return OMITTED;
    const copy = { ...body };
    if (copy.providerOptions !== undefined) {
      if (!object(copy.providerOptions)) return OMITTED;
      const options = { ...copy.providerOptions };
      if (options.gateway !== undefined) {
        if (!object(options.gateway)) return OMITTED;
        const gateway = { ...options.gateway };
        if (Object.hasOwn(gateway, "byok")) gateway.byok = "[REDACTED]";
        options.gateway = gateway;
      }
      copy.providerOptions = options;
    }
    const encoded = JSON.stringify(copy);
    if (new TextEncoder().encode(encoded).length > MAX_CAPTURE_BYTES) return OMITTED;
    return JSON.parse(encoded);
  } catch {
    return OMITTED;
  }
}
