import type { ServerResponse } from "node:http";

export function messagesReply(body: Record<string, unknown>, response: ServerResponse): void {
  const refusal = JSON.stringify(body.messages).includes("refuse");
  const reason = refusal ? "content_filter" : "tool_calls";
  const calls = [
    { id: "a", type: "function", function: { name: "weather", arguments: '{"city":"Oslo"}' } },
    { id: "b", type: "function", function: { name: "weather", arguments: '{"city":"Rome"}' } },
  ];
  if (body.stream !== true) {
    response.writeHead(200, { "Content-Type": "application/json" });
    response.end(JSON.stringify({
      id: "messages-test", object: "chat.completion", created: 1, model: "backend-private",
      choices: [{ index: 0, message: refusal ? { role: "assistant", content: "Cannot help." } : { role: "assistant", content: null, tool_calls: calls }, finish_reason: reason }],
    }));
    return;
  }
  response.writeHead(200, { "Content-Type": "text/event-stream" });
  const deltas = refusal ? [{ content: "Cannot help." }] : [
    { tool_calls: [{ index: 0, ...calls[0], function: { name: "weather", arguments: '{"city":' } }] },
    { tool_calls: [{ index: 1, ...calls[1] }] },
    { tool_calls: [{ index: 0, function: { arguments: '"Oslo"}' } }] },
  ];
  for (const delta of deltas) {
    response.write(`data: ${JSON.stringify({ id: "messages-test", object: "chat.completion.chunk", created: 1, model: "backend-private", choices: [{ index: 0, delta, finish_reason: null }] })}\n\n`);
  }
  response.write(`data: ${JSON.stringify({ id: "messages-test", object: "chat.completion.chunk", created: 1, model: "backend-private", choices: [{ index: 0, delta: {}, finish_reason: reason }] })}\n\n`);
  response.end("data: [DONE]\n\n");
}
