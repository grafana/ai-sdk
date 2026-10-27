import type { ServerResponse } from "node:http";

// Synthetic native boundary evidence; these are not recorded provider fixtures.
export function nativeApprovalReply(body: Record<string, unknown>, response: ServerResponse, failAfterApproval = false): void {
  const continued = (body.input as any[]).some(item => item.type === "mcp_approval_response");
  const output: any[] = continued
    ? [{ id: "msg-done", type: "message", role: "assistant", status: "completed", content: [{ type: "output_text", text: "done", annotations: [], logprobs: [] }] }]
    : [0, 1].map(index => ({ type: "mcp_approval_request", id: `approval-${index}`, name: "lookup", server_label: "srv", arguments: JSON.stringify({ index }) }));
  if (continued && !JSON.stringify(body.input).includes("third-turn")) for (const approval of (body.input as any[]).filter(item=>item.type==="mcp_approval_response" && item.approve)) output.unshift({type:"mcp_call",id:`result-${approval.approval_request_id}`,approval_request_id:approval.approval_request_id,name:"lookup",server_label:"srv",arguments:"{}",output:"provider-result",error:null});
  if (!continued && JSON.stringify(body.input).includes("approval-local")) output.push({type:"function_call",id:"fc-local",call_id:"local-call",name:"local",arguments:"{}",status:"completed"});
  const completed = { id: "resp-approval", object: "response", created_at: 1, model: "backend-private", status: "completed", output, usage: { input_tokens: 2, output_tokens: 3, total_tokens: 5 } };
  const stream = body.stream === true;
  response.writeHead(200, { "Content-Type": stream ? "text/event-stream" : "application/json" });
  if (!stream) { response.end(JSON.stringify(completed)); return; }
  let sequence_number = 0;
  const event = (value: Record<string, unknown>) => response.write(`event: ${value.type}\ndata: ${JSON.stringify({ ...value, sequence_number: sequence_number++ })}\n\n`);
  event({ type: "response.created", response: { ...completed, status: "in_progress", output: [], usage: null } });
  output.forEach((item, output_index) => {
    event({ type: "response.output_item.added", output_index, item: item.type === "message" ? { ...item, content: [] } : item });
    if (item.type === "message") event({ type: "response.output_text.delta", output_index, content_index: 0, item_id: item.id, delta: "done", logprobs: [] });
    event({ type: "response.output_item.done", output_index, item });
  });
  if(failAfterApproval && !continued) event({type:"response.failed",response:{...completed,status:"failed",error:{code:"server_error",message:"synthetic failure after approval"}}});
  else event({ type: "response.completed", response: completed });
  response.end();
}
