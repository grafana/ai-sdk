import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { isDeepStrictEqual } from "node:util";
import Ajv2020 from "ajv/dist/2020.js";
import { validateBaselineMetadata } from "./baseline.ts";
import { valueAtPointer } from "./classification.ts";
import { requestCoverage } from "./request-coverage.ts";
import type { SemanticRequestsArtifact } from "./artifacts.ts";
import type { SemanticRequest } from "./capture.ts";

const packageRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const schemaPath = join(packageRoot, "schema", "providerwire-v4-request.schema.json");
export function createRequestBodyValidator() {
  const schema = JSON.parse(readFileSync(schemaPath, "utf8"));
  return new Ajv2020({ allErrors: true, strict: true }).compile(schema);
}

export function validateEvidence(artifact: SemanticRequestsArtifact): string[] {
  const errors = validateBaselineMetadata("semantic request artifact", artifact.baseline);
  const scenarios = new Map<string, (typeof artifact.scenarios)[number]>();
  const validateBody = createRequestBodyValidator();
  for (const scenario of artifact.scenarios) {
    if (scenarios.has(scenario.name)) {
      errors.push(`duplicate scenario ${scenario.name}`);
    }
    scenarios.set(scenario.name, scenario);
    if (scenario.requests.length === 0) {
      errors.push(`${scenario.name} captured no requests`);
    }
    for (const request of scenario.requests) {
      errors.push(...validateRequest(scenario.name, request, validateBody));
    }
  }

  for (const category of requestCoverage) {
    for (const [memberName, entry] of Object.entries(category.members)) {
      if ("exclusion" in entry) {
        continue;
      }
      const scenario = scenarios.get(entry.scenario);
      if (!scenario) {
        errors.push(`${category.id}.${memberName} references missing scenario ${entry.scenario}`);
        continue;
      }
      const observation = valueAtPointer(scenario, entry.path);
      if (!observation.found) {
        errors.push(
          `${category.id}.${memberName} did not observe ${entry.path} in ${entry.scenario}`,
        );
      } else if ("expected" in entry && !isDeepStrictEqual(observation.value, entry.expected)) {
        errors.push(
          `${category.id}.${memberName} expected ${JSON.stringify(entry.expected)} at ${entry.path}, got ${JSON.stringify(observation.value)}`,
        );
      }
    }
  }

  errors.push(...validateHeaderComposition(scenarios));
  errors.push(...validateSchemaPreservation(scenarios));
  errors.push(...validateMultiStepScenario(scenarios));
  return errors;
}

function validateRequest(
  scenario: string,
  request: SemanticRequest,
  validateBody: ReturnType<typeof createRequestBodyValidator>,
): string[] {
  const errors: string[] = [];
  if (request.method !== "POST") {
    errors.push(`${scenario} request ${request.sequence} method is ${request.method}`);
  }
  if (request.path !== "/language-model") {
    errors.push(`${scenario} request ${request.sequence} relative path is ${request.path}`);
  }
  if (request.requestPath !== "/api/v1/aisdk/language-model") {
    errors.push(`${scenario} request ${request.sequence} composed path is ${request.requestPath}`);
  }

  if (!validateBody(request.body)) {
    errors.push(
      `${scenario} request ${request.sequence} body schema failed: ${JSON.stringify(validateBody.errors)}`,
    );
  }

  const expectedUnsupported =
    scenario.startsWith("header-exact-content-type") ||
    scenario.startsWith("header-case-content-type");
  if (expectedUnsupported && request.envelope !== "unsupported-reserved-collision") {
    errors.push(`${scenario} must be classified as an unsupported reserved collision`);
  }
  if (!expectedUnsupported && request.envelope !== "supported") {
    errors.push(`${scenario} must satisfy the strict V4 envelope`);
  }
  return errors;
}

function validateHeaderComposition(
  scenarios: Map<string, { requests: SemanticRequest[] }>,
): string[] {
  const errors: string[] = [];
  const call = scenarios.get("header-call")?.requests[0];
  assertEqualPointers(
    errors,
    call?.body,
    "/headers/x-call",
    call?.headers,
    "/x-call",
    "call header body/outer propagation",
  );
  assertEqualPointers(
    errors,
    call?.body,
    "/headers/traceparent",
    call?.headers,
    "/traceparent",
    "arbitrary call header body/outer propagation",
  );

  const observability = scenarios.get("header-observability")?.requests[0];
  const bodyValue = valueAtPointer(observability?.body, "/headers/ai-o11y-deployment-id");
  const outerValue = valueAtPointer(observability?.headers, "/ai-o11y-deployment-id");
  if (!bodyValue.found || !outerValue.found || bodyValue.value === outerValue.value) {
    errors.push("observability composition must preserve the caller body value and replace the outer value");
  }
  return errors;
}

function validateSchemaPreservation(
  scenarios: Map<string, { requests: SemanticRequest[] }>,
): string[] {
  const errors: string[] = [];
  const scenario = scenarios.get("presence-losses");
  assertExpectations(errors, scenario, [
    ["/requests/0/body/responseFormat/schema/$ref", "#/$defs/value", "response JSON Schema ref"],
    [
      "/requests/0/body/responseFormat/schema/$defs/value/type",
      "string",
      "response JSON Schema defs",
    ],
    ["/requests/0/body/tools/0/inputSchema/$ref", "#/$defs/input", "tool JSON Schema ref"],
    [
      "/requests/0/body/tools/0/inputSchema/$defs/input/type",
      "object",
      "tool JSON Schema defs",
    ],
  ]);
  return errors;
}

function validateMultiStepScenario(
  scenarios: Map<string, { requests: SemanticRequest[] }>,
): string[] {
  const errors: string[] = [];
  const scenario = scenarios.get("multi-step-tool");
  if (!scenario || scenario.requests.length !== 2) {
    return [
      `multi-step-tool expected 2 requests, got ${scenario?.requests.length ?? 0}`,
    ];
  }
  assertExpectations(errors, scenario, [
    ["/requests/0/sequence", 1, "first request sequence"],
    ["/requests/1/sequence", 2, "continuation request sequence"],
    [
      "/requests/0/body/prompt/2/content/0/type",
      "tool-approval-response",
      "approval response in initial prompt",
    ],
    [
      "/requests/0/body/prompt/2/content/0/approvalId",
      "approval-1",
      "approval correlation in initial prompt",
    ],
    [
      "/requests/0/body/prompt/2/content/0/approved",
      true,
      "approval decision in initial prompt",
    ],
    [
      "/requests/0/body/prompt/2/content/0/reason",
      "approved",
      "approval reason in initial prompt",
    ],
    [
      "/requests/1/body/prompt/0/content/0/text",
      "continue after the approved provider tool",
      "continuation user content",
    ],
    [
      "/requests/1/body/prompt/3/content/0/input/text",
      "hello",
      "continuation tool-call input",
    ],
    [
      "/requests/1/body/prompt/3/content/0/toolCallId",
      "call-echo-1",
      "continuation tool-call correlation",
    ],
    [
      "/requests/1/body/prompt/3/content/0/toolName",
      "echoTool",
      "continuation tool-call name",
    ],
    [
      "/requests/1/body/prompt/4/content/0/type",
      "tool-result",
      "tool result in continuation prompt",
    ],
    [
      "/requests/1/body/prompt/4/content/0/toolCallId",
      "call-echo-1",
      "continuation tool-result correlation",
    ],
    [
      "/requests/1/body/prompt/4/content/0/toolName",
      "echoTool",
      "continuation tool-result name",
    ],
    [
      "/requests/1/body/prompt/4/content/0/output/value/echoed",
      "hello",
      "executed tool output in continuation prompt",
    ],
  ]);
  return errors;
}

function assertExpectations(
  errors: string[],
  root: unknown,
  expectations: ReadonlyArray<readonly [path: string, expected: unknown, label: string]>,
): void {
  for (const [path, expected, label] of expectations) {
    assertValue(errors, root, path, expected, label);
  }
}

function assertEqualPointers(
  errors: string[],
  leftRoot: unknown,
  leftPath: string,
  rightRoot: unknown,
  rightPath: string,
  label: string,
): void {
  const left = valueAtPointer(leftRoot, leftPath);
  const right = valueAtPointer(rightRoot, rightPath);
  if (!left.found || !right.found || !isDeepStrictEqual(left.value, right.value)) {
    errors.push(`${label}: ${JSON.stringify(left.value)} != ${JSON.stringify(right.value)}`);
  }
}

function assertValue(
  errors: string[],
  root: unknown,
  path: string,
  expected: unknown,
  label: string,
): void {
  const actual = valueAtPointer(root, path);
  if (!actual.found || !isDeepStrictEqual(actual.value, expected)) {
    errors.push(`${label}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual.value)}`);
  }
}
