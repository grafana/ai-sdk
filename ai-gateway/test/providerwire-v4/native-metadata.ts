import type { ServerResponse } from "node:http";

export function nativeMetadataReply(family: "anthropic" | "openai" | "compatible", body: Record<string, unknown>, response: ServerResponse): void {
  const continued = JSON.stringify(body).includes("sunny");
  const stream = body.stream === true;
  response.writeHead(200, { "Content-Type": stream ? "text/event-stream" : "application/json" });
  if (family === "anthropic") {
    const content = continued ? [{ type: "text" as const, text: "done" }] : [
      { type: "thinking" as const, thinking: "thought", signature: "returned-signature" },
      { type: "redacted_thinking" as const, data: "returned-redacted" },
      { type: "tool_use" as const, id: "call-weather", name: "weather", input: { city: "Rio" }, caller: { type: "direct" } },
    ];
    const message = { id: "msg-metadata", type: "message", role: "assistant", model: "backend-private", content, stop_reason: continued ? "end_turn" : "tool_use", stop_sequence: null, usage: { input_tokens: 2, output_tokens: 3 } };
    if (!stream) { response.end(JSON.stringify(message)); return; }
    const event = (value: Record<string, unknown>) => response.write(`event: ${value.type}\ndata: ${JSON.stringify(value)}\n\n`);
    event({ type: "message_start", message: { ...message, content: [], stop_reason: null, usage: { input_tokens: 2, output_tokens: 0 } } });
    content.forEach((block, index) => {
      event({ type: "content_block_start", index, content_block: block.type === "thinking" ? { ...block, thinking: "", signature: "" } : block.type === "text" ? { ...block, text: "" } : block.type === "tool_use" ? { ...block, input: {} } : block });
      if (block.type === "thinking") {
        event({ type: "content_block_delta", index, delta: { type: "thinking_delta", thinking: block.thinking } });
        event({ type: "content_block_delta", index, delta: { type: "signature_delta", signature: block.signature } });
      } else if (block.type === "text") {
        event({ type: "content_block_delta", index, delta: { type: "text_delta", text: block.text } });
      } else if (block.type === "tool_use") {
        event({ type: "content_block_delta", index, delta: { type: "input_json_delta", partial_json: JSON.stringify(block.input) } });
      }
      event({ type: "content_block_stop", index });
    });
    event({ type: "message_delta", delta: { stop_reason: message.stop_reason, stop_sequence: null }, usage: { output_tokens: 3 } });
    event({ type: "message_stop" });
  } else if (family === "openai") {
    const output = continued ? [{ id: "msg-done", type: "message" as const, role: "assistant", status: "completed", content: [{ type: "output_text", text: "done", annotations: [], logprobs: [] }] }] : [
      { id: "msg-metadata", type: "message" as const, role: "assistant", status: "completed", phase: "commentary", content: [{ type: "output_text", text: "plan", annotations: [], logprobs: [] }] },
      { id: "rs-metadata", type: "reasoning" as const, summary: [{ type: "summary_text", text: "thought" }], encrypted_content: "returned-encrypted" },
      { id: "fc-metadata", type: "function_call" as const, status: "completed", call_id: "call-weather", name: "weather", arguments: JSON.stringify({ city: "Rio" }) },
    ];
    const completed = { id: "resp-metadata", object: "response", created_at: 1, model: "backend-private", status: "completed", output, usage: { input_tokens: 2, output_tokens: 3, output_tokens_details: { reasoning_tokens: 1 }, total_tokens: 5 } };
    if (!stream) { response.end(JSON.stringify(completed)); return; }
    let sequence_number = 0;
    const event = (value: Record<string, unknown>) => response.write(`event: ${value.type}\ndata: ${JSON.stringify({ ...value, sequence_number: sequence_number++ })}\n\n`);
    event({ type: "response.created", response: { ...completed, status: "in_progress", output: [], usage: null } });
    output.forEach((item, output_index) => {
      event({ type: "response.output_item.added", output_index, item: item.type === "message" ? { ...item, content: [] } : item.type === "reasoning" ? { ...item, summary: [], encrypted_content: null } : { ...item, arguments: "" } });
      if (item.type === "message") event({ type: "response.output_text.delta", output_index, content_index: 0, item_id: item.id, delta: item.content![0].text, logprobs: [] });
      if (item.type === "reasoning") event({ type: "response.reasoning_summary_text.delta", output_index, summary_index: 0, item_id: item.id, delta: "thought" });
      if (item.type === "function_call") event({ type: "response.function_call_arguments.delta", output_index, item_id: item.id, delta: item.arguments });
      event({ type: "response.output_item.done", output_index, item });
    });
    event({ type: "response.completed", response: completed });
  } else {
    const toolCall = { id: "call-weather", type: "function", function: { name: "weather", arguments: JSON.stringify({ city: "Rio" }) }, extra_content: { google: { thought_signature: "returned-thought-signature" } } };
    const usage = { prompt_tokens: 2, completion_tokens: 3, total_tokens: 5 };
    const finish_reason = continued ? "stop" : "tool_calls";
    const base = { id: "chat-metadata", created: 1, model: "backend-private" };
    if (!stream) { response.end(JSON.stringify({ ...base, object: "chat.completion", choices: [{ index: 0, message: continued ? { role: "assistant", content: "done" } : { role: "assistant", content: null, tool_calls: [toolCall] }, finish_reason }], usage })); return; }
    const chunk = (choices: unknown[], tokens?: unknown) => response.write(`data: ${JSON.stringify({ ...base, object: "chat.completion.chunk", choices, ...(tokens ? { usage: tokens } : {}) })}\n\n`);
    chunk([{ index: 0, delta: continued ? { content: "done" } : { tool_calls: [{ ...toolCall, index: 0 }] }, finish_reason: null }]);
    chunk([{ index: 0, delta: {}, finish_reason }], usage);
    response.write("data: [DONE]\n\n");
  }
  response.end();
}
