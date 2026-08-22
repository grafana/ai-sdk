import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import Ajv2020, { type ErrorObject, type ValidateFunction } from "ajv/dist/2020.js";

const schemaPath = fileURLToPath(
  new URL("../../gateway/providerwire/v4/schema/request.json", import.meta.url),
);
const schema = JSON.parse(readFileSync(schemaPath, "utf8")) as object;
const ajv = new Ajv2020({ allErrors: true, strict: true });

export const validateRequest: ValidateFunction<unknown> = ajv.compile(schema);

export function compileDefinition(name: string): ValidateFunction<unknown> {
  return ajv.compile({ $ref: `${(schema as { $id: string }).$id}#/$defs/${name}` });
}

export function formatValidationErrors(errors: ErrorObject[] | null | undefined): string {
  return (errors ?? [])
    .map((error) => `${error.instancePath || "/"} ${error.message ?? "is invalid"}`)
    .join("; ");
}

export function assertValidRequest(value: unknown, label: string): void {
  if (!validateRequest(value)) {
    throw new Error(`${label}: ${formatValidationErrors(validateRequest.errors)}`);
  }
}
