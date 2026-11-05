import { type UIMessageChunk } from "ai";
import { describe, expect, it } from "vitest";
import { fetchScenario } from "./helpers.js";
import { canonicalUI, readToolStateStream } from "./ui-tool-state-helpers.js";

describe("partial JSON with an initial array minus", () => {
  it("emits the first structured text delta before the negative digit", async () => {
    const parsed = await readToolStateStream(await fetchScenario("initial-array-minus"));
    expect(parsed.chunks.filter(chunk => chunk.type === "text-delta").map(chunk => chunk.delta)).toEqual([
      '{"series":"temp","values":[-', "3,", "4]}",
    ]);
    expect(parsed.snapshots.at(-1)?.parts).toContainEqual({
      type: "text", text: '{"series":"temp","values":[-3,4]}', state: "done",
    });
  });

  it.each([false, true])("retains tool input at the minus boundary (dynamic: %s)", async dynamic => {
    const chunks: UIMessageChunk[] = [
      { type: "start", messageId: "m" },
      { type: "tool-input-start", toolCallId: "c", toolName: "weather", dynamic },
      { type: "tool-input-delta", toolCallId: "c", inputTextDelta: '{"series":"temp","values":[' },
      { type: "tool-input-delta", toolCallId: "c", inputTextDelta: "-" },
      { type: "tool-input-delta", toolCallId: "c", inputTextDelta: "3," },
    ];
    const body = JSON.stringify({ chunks, ignoreIncomplete: true });
    const options = { headers: { "content-type": "application/json" }, body };
    const reference = await readToolStateStream(await fetchScenario("ui-tool-state-chunks", options));
    expect(reference.chunks).toEqual(chunks);
    const response = await fetchScenario("ui-tool-state-persistence", options);
    expect(response.ok).toBe(true);
    const go = await response.json();
    expect(canonicalUI(go.snapshots)).toEqual(canonicalUI(reference.snapshots));
    expect(go.snapshots.at(-2).parts[0]).toMatchObject({ state: "input-streaming", input: { series: "temp", values: [] } });
    expect(go.snapshots.at(-1).parts[0]).toMatchObject({ state: "input-streaming", input: { series: "temp", values: [-3] } });
  });
});
