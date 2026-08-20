import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  generateSemanticRequestsArtifact,
  type SemanticRequestsArtifact,
} from "./artifacts.ts";
import { validateEvidence } from "./validate-evidence.ts";

function scenario(artifact: SemanticRequestsArtifact, name: string) {
  const capture = artifact.scenarios.find((candidate) => candidate.name === name);
  assert.ok(capture, `missing scenario ${name}`);
  return capture;
}

describe("evidence observations", () => {
  it("rejects exact header values even when relational checks still pass", async () => {
    const artifact = await generateSemanticRequestsArtifact();
    const request = scenario(artifact, "header-call").requests[0];
    const body = request.body as { headers: Record<string, string> };
    body.headers.traceparent = "wrong";
    request.headers.traceparent = "wrong";

    const errors = validateEvidence(artifact);
    assert.ok(errors.some((error) => error.includes("gateway-transform.call-trace-body")));
    assert.ok(errors.some((error) => error.includes("gateway-transform.call-trace-outer")));
  });

  it("rejects multi-step approval and continuation correlation drift", async () => {
    const artifact = await generateSemanticRequestsArtifact();
    const capture = scenario(artifact, "multi-step-tool");
    const initialPrompt = (capture.requests[0].body as { prompt: Array<{ content: unknown[] }> })
      .prompt;
    const approval = initialPrompt[2].content[0] as { approvalId: string };
    approval.approvalId = "wrong";

    const continuationPrompt = (
      capture.requests[1].body as { prompt: Array<{ content: unknown[] }> }
    ).prompt;
    const toolResult = continuationPrompt[4].content[0] as { toolCallId: string };
    toolResult.toolCallId = "wrong";

    const errors = validateEvidence(artifact);
    assert.ok(errors.some((error) => error.includes("approval correlation")));
    assert.ok(errors.some((error) => error.includes("tool-result correlation")));
  });
});
