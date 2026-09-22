import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const toolsDirectory = dirname(fileURLToPath(import.meta.url));
const repositoryRoot = resolve(toolsDirectory, "..", "..", "..");

export interface GatewayContractInventory {
  types: Record<string, string>;
  constants: Record<string, Record<string, string>>;
}

export interface GatewayContractEvidence extends GatewayContractInventory {
  upstreamCommit: string;
  packages: Record<string, string>;
  classifications: Record<string, "mapped" | "omitted" | "rejected">;
}

export function collectGatewayContract(providerDirectory = join(repositoryRoot, "provider")): GatewayContractInventory {
  return JSON.parse(execFileSync("go", [
    "run", join(toolsDirectory, "gateway-contract-inventory", "main.go"), providerDirectory,
  ], { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"], env: { ...process.env, GOWORK: "off" } })) as GatewayContractInventory;
}

export function readGatewayContractEvidence(): GatewayContractEvidence {
  return JSON.parse(readFileSync(join(toolsDirectory, "gateway-client-contract.json"), "utf8")) as GatewayContractEvidence;
}

export function validateGatewayContract(
  baseline: { upstream?: { commit?: string }; packages?: Record<string, unknown> },
  evidence: GatewayContractEvidence,
  actual: GatewayContractInventory,
): string[] {
  const errors: string[] = [];
  const review = "review Gateway client mapping, differential evidence, and PARITY.md before updating gateway-client-contract.json";
  if (baseline.upstream?.commit !== evidence.upstreamCommit) {
    errors.push(`Gateway client evidence upstream commit changed; ${review}`);
  }
  for (const name of ["@ai-sdk/gateway", "@ai-sdk/provider", "@ai-sdk/provider-utils"]) {
    if (baseline.packages?.[name] !== evidence.packages[name]) {
      errors.push(`Gateway client evidence ${name} pin changed; ${review}`);
    }
  }
  for (const name of new Set([...Object.keys(evidence.types), ...Object.keys(actual.types)])) {
    if (evidence.types[name] !== actual.types[name]) {
      errors.push(`Gateway client provider.${name} declaration changed; ${review}`);
    }
  }
  for (const type of new Set([...Object.keys(evidence.constants), ...Object.keys(actual.constants)])) {
    const expectedConstants = evidence.constants[type] ?? {};
    const actualConstants = actual.constants[type] ?? {};
    for (const name of new Set([...Object.keys(expectedConstants), ...Object.keys(actualConstants)])) {
      if (expectedConstants[name] !== actualConstants[name]) {
        errors.push(`Gateway client provider.${type} discriminator ${name} changed; ${review}`);
      }
      if (actualConstants[name] !== undefined && !["mapped", "omitted", "rejected"].includes(evidence.classifications[name])) {
        errors.push(`Gateway client provider.${type} discriminator ${name} has no mapping classification; ${review}`);
      }
    }
  }
  return errors;
}
