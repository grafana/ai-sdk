import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { describe, it } from "node:test";
import Anthropic from "@anthropic-ai/sdk";

import { TOKEN, Gateway, SyntheticAnthropic, binaries } from "./test-support.js";

const execFileAsync = promisify(execFile);

describe("official Anthropic SDKs against the local Gateway command", () => {
	it("maps TypeScript SDK unary, tool continuation, stream lifecycle, errors, auth, and routing", async () => {
		const upstream = await SyntheticAnthropic.start();
		const gateway = await Gateway.start(upstream.url);
		const client = new Anthropic({ apiKey: TOKEN, baseURL: gateway.url, maxRetries: 0 });
		try {
			const unary = await client.messages.create({ model: "assistant", max_tokens: 64, system: "Be concise", messages: [{ role: "user", content: "typescript unary" }] });
			assert.equal(unary.content[0]?.type, "text");
			assert.equal(unary.content[0]?.text, "synthetic answer");
			assert.equal(unary.stop_reason, "end_turn");
			assert.equal(unary.model, "grafana/assistant");
			assert.equal(unary.usage.input_tokens, 7);
			assert.equal(unary.usage.output_tokens, 3);
			assert.equal(unary.usage.cache_read_input_tokens, 2);
			assert.equal(unary.usage.cache_creation_input_tokens, 1);

			const tools = [{ name: "weather", description: "Get weather", input_schema: { type: "object" as const, properties: { city: { type: "string" } }, required: ["city"] }, input_examples: [{ city: "Oslo" }] }];
			const first = await client.messages.create({ model: "assistant", max_tokens: 64, tools, messages: [{ role: "user", content: "typescript tool" }] });
			const call = first.content.find((part) => part.type === "tool_use");
			assert.ok(call && call.type === "tool_use");
			assert.deepEqual(call.input, { city: "Oslo" });
			assert.deepEqual(upstream.requests[1]?.body.tools[0]?.input_examples, [{ city: "Oslo" }]);
			const continued = await client.messages.create({ model: "assistant", max_tokens: 64, tools, messages: [
				{ role: "user", content: "typescript tool" },
				{ role: "assistant", content: [{ type: "tool_use", id: call.id, name: call.name, input: call.input }] },
				{ role: "user", content: [{ type: "tool_result", tool_use_id: call.id, content: "snow" }] },
			] });
			assert.equal(continued.content[0]?.type, "text");
			assert.equal(continued.content[0]?.text, "tool result received");
			const emptyResult = await client.messages.create({ model: "assistant", max_tokens: 64, tools, messages: [
				{ role: "user", content: "typescript tool" },
				{ role: "assistant", content: [{ type: "tool_use", id: call.id, name: call.name, input: call.input }] },
				{ role: "user", content: [{ type: "tool_result", tool_use_id: call.id }] },
			] });
			assert.equal(emptyResult.content[0]?.type, "text");
			assert.equal(emptyResult.content[0]?.text, "tool result received");

			const toolStream = client.messages.stream({ model: "assistant", max_tokens: 64, tools, messages: [{ role: "user", content: "typescript stream tool" }] });
			for await (const _event of toolStream) {}
			const streamedTool = await toolStream.finalMessage();
			const streamedCall = streamedTool.content.find((part) => part.type === "tool_use");
			assert.ok(streamedCall && streamedCall.type === "tool_use");
			assert.deepEqual(streamedCall.input, { city: "Oslo" });

			const mixedToolStream = client.messages.stream({ model: "assistant", max_tokens: 64, tools, messages: [{ role: "user", content: "typescript mixed stream tool" }] });
			const mixedEvents: Array<{ type: string; index?: number }> = [];
			for await (const event of mixedToolStream) mixedEvents.push(event as { type: string; index?: number });
			assert.deepEqual(mixedEvents.filter((event) => event.type === "content_block_start").map((event) => event.index), [0, 1]);
			const mixedMessage = await mixedToolStream.finalMessage();
			assert.deepEqual(mixedMessage.content.map((part) => part.type), ["text", "tool_use"]);
			const mixedCall = mixedMessage.content[1];
			assert.ok(mixedCall?.type === "tool_use");
			assert.deepEqual(mixedCall.input, { city: "Oslo" });

			const stream = client.messages.stream({ model: "assistant", max_tokens: 64, messages: [{ role: "user", content: "typescript stream" }] });
			const eventTypes: string[] = [];
			for await (const event of stream) eventTypes.push(event.type);
			assert.deepEqual(eventTypes, ["message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"]);
			const streamed = await stream.finalMessage();
			assert.equal(streamed.content[0]?.type, "text");
			assert.equal(streamed.content[0]?.text, "synthetic stream");
			assert.equal(streamed.id, "msg_synthetic");
			assert.equal(streamed.usage.input_tokens, 7);
			assert.equal(streamed.usage.output_tokens, 3);
			assert.equal(streamed.usage.cache_read_input_tokens, 2);
			assert.equal(streamed.usage.cache_creation_input_tokens, 1);

			await assert.rejects(client.messages.create({ model: "assistant", max_tokens: 64, messages: [{ role: "user", content: "provider failure" }] }), (error: unknown) => {
				assert.equal((error as { status?: number }).status, 502);
				assert.match((error as Error).message, /Provider request failed/);
				return true;
			});
			const badAuth = new Anthropic({ apiKey: "invalid-gateway-token", baseURL: gateway.url, maxRetries: 0 });
			await assert.rejects(badAuth.messages.create({ model: "assistant", max_tokens: 64, messages: [{ role: "user", content: "auth" }] }), (error: unknown) => (error as { status?: number }).status === 401);
			const bearerClient = new Anthropic({ authToken: TOKEN, baseURL: gateway.url, maxRetries: 0 });
			const bearerReply = await bearerClient.messages.create({ model: "assistant", max_tokens: 64, messages: [{ role: "user", content: "bearer auth" }] });
			assert.equal(bearerReply.content[0]?.type, "text");
			const beforeUnknown = upstream.requests.length;
			await assert.rejects(client.messages.create({ model: "unknown", max_tokens: 64, messages: [{ role: "user", content: "routing" }] }), (error: unknown) => (error as { status?: number }).status === 404);
			assert.equal(upstream.requests.length, beforeUnknown);
			for (const listener of [gateway.cloudURL, gateway.operationalURL]) {
				const response = await fetch(`${listener}/v1/messages`, { method: "POST", headers: { "X-Scope-OrgID": "123" }, body: "{}" });
				assert.equal(response.status, 404);
			}
			assert.equal(upstream.requests.length, beforeUnknown);
			assert.equal(upstream.violations.length, 0, upstream.violations.join(", "));
		} finally {
			await gateway.stop();
			await upstream.stop();
		}
	});

	it("runs unary, client-tool continuation, and incremental stream through the official Go SDK", async () => {
		const upstream = await SyntheticAnthropic.start();
		const gateway = await Gateway.start(upstream.url);
		try {
			const { stdout: output } = await execFileAsync(binaries.goClient, [gateway.url, TOKEN], { encoding: "utf8", timeout: 30_000 });
			const summary = JSON.parse(output) as { unary_text: string; unary_stop_reason: string; unary_input_tokens: number; unary_output_tokens: number; tool_text: string; stream_text: string; stream_id: string; stream_usage: { input_tokens: number; output_tokens: number; cache_read_input_tokens: number; cache_creation_input_tokens: number }; stream_events: string[] };
			assert.equal(summary.unary_text, "synthetic answer");
			assert.equal(summary.unary_stop_reason, "end_turn");
			assert.equal(summary.unary_input_tokens, 7);
			assert.equal(summary.unary_output_tokens, 3);
			assert.equal(summary.tool_text, "tool result received");
			assert.equal(summary.stream_text, "synthetic stream");
			assert.equal(summary.stream_id, "msg_synthetic");
			assert.equal(summary.stream_usage.input_tokens, 7);
			assert.equal(summary.stream_usage.output_tokens, 3);
			assert.equal(summary.stream_usage.cache_read_input_tokens, 2);
			assert.equal(summary.stream_usage.cache_creation_input_tokens, 1);
			assert.equal(summary.stream_events.length, 6);
			assert.ok(summary.stream_events[0]?.includes("MessageStartEvent"));
			assert.ok(summary.stream_events.at(-1)?.includes("MessageStopEvent"));
			assert.equal(upstream.requests.length, 4);
			assert.equal(upstream.requests[0]?.path, "/v1/messages");
			assert.equal(upstream.requests[0]?.body.max_tokens, 64);
			assert.equal(upstream.requests[1]?.body.tools[0]?.name, "weather");
			const continuation = upstream.requests[2]?.body.messages as Array<{ role: string; content: Array<{ type: string; tool_use_id?: string; id?: string }> }>;
			assert.ok(continuation.some((message) => message.content.some((part) => part.type === "tool_use" && part.id === "toolu_synthetic")));
			assert.ok(continuation.some((message) => message.content.some((part) => part.type === "tool_result" && part.tool_use_id === "toolu_synthetic")));
			assert.equal(upstream.violations.length, 0, upstream.violations.join(", "));
		} finally {
			await gateway.stop();
			await upstream.stop();
		}
	});

	it("accepts official SDK API key and bearer authentication with configured static keys", async () => {
		const upstream = await SyntheticAnthropic.start();
		const gateway = await Gateway.start(upstream.url, true);
		try {
			for (const credentials of [{ apiKey: TOKEN }, { authToken: TOKEN }]) {
				const client = new Anthropic({ ...credentials, baseURL: gateway.url, maxRetries: 0 });
				const reply = await client.messages.create({ model: "assistant", max_tokens: 64, messages: [{ role: "user", content: "static auth" }] });
				assert.equal(reply.content[0]?.type, "text");
			}
			assert.equal(upstream.requests.length, 2);
			assert.deepEqual(upstream.violations, []);
		} finally {
			await gateway.stop();
			await upstream.stop();
		}
	});

	it("cancels provider streaming when an official SDK stream is aborted", async () => {
		const upstream = await SyntheticAnthropic.start();
		upstream.holdStream = true;
		const gateway = await Gateway.start(upstream.url);
		const controller = new AbortController();
		const client = new Anthropic({ apiKey: TOKEN, baseURL: gateway.url, maxRetries: 0 });
		try {
			const stream = client.messages.stream({ model: "assistant", max_tokens: 64, messages: [{ role: "user", content: "cancel this stream" }] }, { signal: controller.signal });
			for await (const _event of stream) {
				controller.abort();
				break;
			}
			await upstream.waitForCancellation();
		} finally {
			await gateway.stop();
			await upstream.stop();
		}
	});
});
