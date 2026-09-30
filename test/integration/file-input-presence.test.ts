import { convertToModelMessages, validateUIMessages, type UIMessage } from "ai";
import { describe, expect, it } from "vitest";

import { fetchScenario } from "./helpers.js";

describe("ordinary UI file input filename presence", () => {
  it("matches pinned UI conversion and the Go Gateway client projection", async () => {
    const messages: UIMessage[] = [];
    for (const role of ["user", "assistant"] as const) {
      for (const filename of [undefined, "", "report.pdf"]) {
        messages.push({
          id: `${role}-${filename === undefined ? "absent" : filename === "" ? "empty" : "named"}`,
          role,
          parts: [{
            type: "file",
            mediaType: "application/pdf",
            url: "https://example.test/report.pdf",
            ...(filename === undefined ? {} : { filename }),
          }],
        });
      }
      messages.push({
        id: `${role}-reference`, role,
        parts: [{
          type: "file", mediaType: "application/pdf", filename: "",
          url: "https://example.test/ignored.pdf",
          providerReference: { anthropic: "file-1" },
        }],
      });
    }

    const validated = await validateUIMessages({ messages });
    const converted = await convertToModelMessages(validated.map(({ id: _id, ...message }) => message));
    const expected = JSON.parse(JSON.stringify(converted));
    const response = await fetchScenario("file-input-presence", {
      headers: { "content-type": "application/json" },
      body: JSON.stringify(messages),
    });
    expect(response.status).toBe(200);
    const go = await response.json();
    expect(go.uiMessages).toEqual(messages);
    expect(go.modelMessages).toEqual(expected);
    expect(go.request.prompt).toEqual(expected);
    for (const [index, message] of expected.entries()) {
      const file = message.content[0];
      const projected = go.request.prompt[index].content[0];
      if (index % 4 === 3) {
        expect(file.data).toEqual({ type: "reference", reference: { anthropic: "file-1" } });
        expect(projected.filename).toBe("");
      } else {
        expect(file.data).toEqual({ type: "url", url: "https://example.test/report.pdf" });
        const position = index % 4;
        expect("filename" in projected).toBe(position !== 0);
        if (position !== 0) expect(projected.filename).toBe(position === 1 ? "" : "report.pdf");
      }
    }
  });
});
