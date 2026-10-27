import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { readFileSync } from "node:fs";
import { createServer, type ServerResponse } from "node:http";
import { resolve } from "node:path";
import { describe, it } from "node:test";
import Anthropic from "@anthropic-ai/sdk";
import { binaries, Gateway, TOKEN, UPSTREAM_KEY } from "./test-support.js";

type Event = Record<string, any>;
type Observation = { message?: Anthropic.Message; events: Event[]; error?: string; status?: number; errorBody?: unknown };
type Client = "typescript" | "go";
const fixtureRoot = resolve(import.meta.dirname, "../../../test/conformance/anthropic");
const request: Anthropic.MessageCreateParamsNonStreaming = {
	model: "assistant", max_tokens: 1024,
	messages: [{ role: "user", content: "fixture replay" }],
};

const replays = [
	{ name: "recorded text", files: ["recorded/simple-text/input.chunks.txt"] },
	{ name: "recorded tool continuation", files: ["recorded/tool-call/input-1.chunks.txt", "recorded/tool-call/input-2.chunks.txt"] },
	{ name: "recorded parallel tool continuation", files: ["recorded/parallel-tool-calls/input-1.chunks.txt", "recorded/parallel-tool-calls/input-2.chunks.txt"] },
	{ name: "upstream late input usage", files: ["upstream/message-delta-input-tokens/input.chunks.txt"] },
	{ name: "upstream empty tool arguments", files: ["upstream/tool-call-no-args/input.chunks.txt"] },
];

describe("Messages conformance: unchanged provider fixtures and pinned official SDKs", () => {
	for (const fixture of replays) {
		for (const sdk of ["typescript", "go"] as const) {
			it(`${sdk}: ${fixture.name} matches direct SDK replay`, { timeout: 30_000 }, async () => {
				const inputs = fixture.files.map((file) => readFileSync(resolve(fixtureRoot, file), "utf8"));
				const upstream = await ReplayServer.start((response, _body, index) => sendLines(response, inputs[index % inputs.length]!));
				let gateway: Gateway | undefined;
				try {
					gateway = await Gateway.start(upstream.url);
					const direct = await conversation(sdk, upstream.url, UPSTREAM_KEY, inputs.length);
					const adapted = await conversation(sdk, gateway.url, TOKEN, inputs.length);
					assert.deepEqual(adapted.map(project), direct.map(project));
					for (const result of adapted) assertLifecycle(result.events);
					assert.equal(upstream.requests.length, inputs.length * 2);
					assert.deepEqual(upstream.violations, []);
					if (inputs.length > 1) {
						const calls = adapted[0]!.message!.content.filter((part) => part.type === "tool_use");
						const history = upstream.requests.at(-1)!.messages as Array<{ role: string; content: Event[] }>;
						const results = history.filter((message) => message.role === "user").flatMap((message) => message.content).filter((part) => part.type === "tool_result");
						assert.deepEqual(results.map((part) => ({ id: part.tool_use_id, text: typeof part.content === "string" ? part.content : part.content.map((item: Event) => item.text).join("") })), calls.map((call) => ({ id: call.id, text: "synthetic client tool result" })));
						const replayedCalls = history.filter((message) => message.role === "assistant").flatMap((message) => message.content).filter((part) => part.type === "tool_use");
						assert.deepEqual(replayedCalls.map((part) => ({ id: part.id, name: part.name, input: part.input })), calls.map((call) => ({ id: call.id, name: call.name, input: call.input })));
					}
				} finally { await gateway?.stop(); await upstream.stop(); }
			});
		}
	}
});

describe("Messages conformance: explicitly synthetic boundary cases", () => {
	for (const sdk of ["typescript", "go"] as const) {
		for (const stop of ["end_turn", "max_tokens", "stop_sequence", "tool_use"] as const) {
			it(`${sdk}: unary ${stop} response and cache usage`, async () => {
				const expected = {
					id: "msg_synthetic_contract", type: "message", role: "assistant", model: "backend-model",
					content: stop === "tool_use" ? [{ type: "tool_use", id: "call", name: "weather", input: { city: "Oslo" } }] : [{ type: "text", text: "synthetic" }],
					stop_reason: stop, stop_sequence: null,
					usage: { input_tokens: 7, output_tokens: 3, cache_read_input_tokens: 2, cache_creation_input_tokens: 1 },
				};
				const upstream = await ReplayServer.start((response) => { response.setHeader("Content-Type", "application/json"); response.end(JSON.stringify(expected)); });
				let gateway: Gateway | undefined;
				try {
					gateway = await Gateway.start(upstream.url);
					const direct = await observe(sdk, upstream.url, UPSTREAM_KEY, request, false);
					const adapted = await observe(sdk, gateway.url, TOKEN, request, false);
					assert.deepEqual(project(adapted), project(direct));
					assert.equal(adapted.message!.model, "grafana/assistant");
				} finally { await gateway?.stop(); await upstream.stop(); }
			});
		}
		it(`${sdk}: fixed pre-stream provider errors`, async () => {
			const upstream = await ReplayServer.start((response) => {
				response.writeHead(503, { "Content-Type": "application/json" });
				response.end(JSON.stringify({ type: "error", error: { type: "api_error", message: "synthetic-private-provider-secret" } }));
			});
			let gateway: Gateway | undefined;
			try {
				gateway = await Gateway.start(upstream.url);
				for (const stream of [false, true]) {
					const result = await observe(sdk, gateway.url, TOKEN, request, stream);
					assert.equal(result.status, 502);
					assert.deepEqual(result.errorBody, { type: "error", error: { type: "api_error", message: "Provider request failed" } });
					assert.equal(result.message, undefined);
					assert.deepEqual(result.events, []);
				}
			} finally { await gateway?.stop(); await upstream.stop(); }
		});
		it(`${sdk}: error after content never completes successfully`, async () => {
			const upstream = await ReplayServer.start((response) => {
				response.setHeader("Content-Type", "text/event-stream");
				for (const event of syntheticPrefix()) sendEvent(response, event);
				sendEvent(response, { type: "error", error: { type: "api_error", message: "synthetic-private-provider-secret" } });
				response.end();
			});
			let gateway: Gateway | undefined;
			try {
				gateway = await Gateway.start(upstream.url);
				const result = await observe(sdk, gateway.url, TOKEN, request, true);
				assert.ok(result.error);
				assert.deepEqual(result.errorBody, { type: "error", error: { type: "api_error", message: "Provider request failed" } });
				assert.equal(result.message, undefined);
				assert.ok(result.events.some((event) => event.type === "content_block_delta"));
				assert.ok(!result.events.some((event) => event.type === "message_stop"));
				assert.ok(!result.error.includes("synthetic-private-provider-secret"));
				assert.equal(upstream.requests.length, 1, "must not replay after output commitment");
			} finally { await gateway?.stop(); await upstream.stop(); }
		});
	}
	it("flushes content before provider completion", { timeout: 10_000 }, async () => {
		let release!: () => void;
		const gate = new Promise<void>((resolveGate) => { release = resolveGate; });
		let completed = false;
		const upstream = await ReplayServer.start(async (response) => {
			response.setHeader("Content-Type", "text/event-stream");
			for (const event of syntheticPrefix()) sendEvent(response, event);
			await gate;
			completed = true;
			sendEvent(response, { type: "content_block_stop", index: 0 });
			sendEvent(response, { type: "message_delta", delta: { stop_reason: "end_turn", stop_sequence: null }, usage: { output_tokens: 1 } });
			sendEvent(response, { type: "message_stop" });
			response.end();
		});
		let gateway: Gateway | undefined;
		try {
			gateway = await Gateway.start(upstream.url);
			const client = new Anthropic({ apiKey: TOKEN, baseURL: gateway.url, maxRetries: 0 });
			const stream = client.messages.stream(request, { signal: AbortSignal.timeout(8_000) });
			let sawDelta = false;
			for await (const event of stream) {
				if (event.type === "content_block_delta") { assert.equal(completed, false); sawDelta = true; release(); }
			}
			assert.equal(sawDelta, true);
			assert.equal((await stream.finalMessage()).stop_reason, "end_turn");
		} finally { release(); await gateway?.stop(); await upstream.stop(); }
	});
});

async function conversation(sdk: Client, url: string, key: string, turns: number): Promise<Observation[]> {
	const params = structuredClone(request);
	params.tools = ["get_weather", "updateIssueList"].map((name) => ({ name, input_schema: { type: "object" } }));
	const results: Observation[] = [];
	for (let turn = 0; turn < turns; turn++) {
		const result = await observe(sdk, url, key, params, true);
		assert.equal(result.error, undefined);
		assert.ok(result.message);
		results.push(result);
		const content = result.message.content.map((part): Anthropic.ContentBlockParam => {
			if (part.type === "text") return { type: "text", text: part.text };
			assert.equal(part.type, "tool_use");
			return { type: "tool_use", id: part.id, name: part.name, input: part.input };
		});
		params.messages.push({ role: "assistant", content });
		params.messages.push({ role: "user", content: content.filter((part) => part.type === "tool_use").map((part) => ({ type: "tool_result" as const, tool_use_id: part.id, content: "synthetic client tool result" })) });
	}
	return results;
}

async function observe(sdk: Client, url: string, key: string, params: Anthropic.MessageCreateParamsNonStreaming, streaming: boolean): Promise<Observation> {
	if (sdk === "go") return new Promise((resolveResult, reject) => {
		const child = execFile(binaries.goClient, [url, key, "conformance"], { timeout: 15_000 }, (error, stdout) => {
			if (error) { reject(error); return; }
			try { resolveResult(JSON.parse(stdout) as Observation); } catch (error) { reject(error); }
		});
		child.stdin!.end(JSON.stringify({ stream: streaming, request: params }));
	});
	const client = new Anthropic({ apiKey: key, baseURL: url, maxRetries: 0 });
	const result: Observation = { events: [] };
	try {
		if (streaming) {
			const stream = client.messages.stream(params);
			for await (const event of stream) result.events.push(event);
			result.message = await stream.finalMessage();
		} else result.message = await client.messages.create(params);
	} catch (error) {
		result.error = String(error);
		if (error instanceof Anthropic.APIError) { result.status = error.status; result.errorBody = error.error; }
	}
	return result;
}

function project(result: Observation) {
	assert.equal(result.error, undefined);
	assert.ok(result.message);
	const message = result.message;
	return {
		id: message.id, type: message.type, role: message.role,
		content: message.content.map((part) => {
			if (part.type === "text") return { type: part.type, text: part.text };
			assert.equal(part.type, "tool_use");
			return { type: part.type, id: part.id, name: part.name, input: part.input };
		}),
		stop_reason: message.stop_reason, stop_sequence: message.stop_sequence || null,
		usage: {
			input_tokens: message.usage.input_tokens, output_tokens: message.usage.output_tokens,
			cache_read_input_tokens: message.usage.cache_read_input_tokens ?? 0,
			cache_creation_input_tokens: message.usage.cache_creation_input_tokens ?? 0,
		},
	};
}

function assertLifecycle(events: Event[]) {
	assert.equal(events[0]?.type, "message_start");
	assert.equal(events.at(-2)?.type, "message_delta");
	assert.equal(events.at(-1)?.type, "message_stop");
	assert.equal(events.filter((event) => event.type === "message_start").length, 1);
	assert.equal(events.filter((event) => event.type === "message_stop").length, 1);
	let index = 0;
	let open = false;
	for (const event of events.slice(1, -2)) {
		if (event.type === "content_block_start") { assert.equal(open, false); assert.equal(event.index, index); open = true; }
		else if (event.type === "content_block_delta") { assert.equal(open, true); assert.equal(event.index, index); }
		else if (event.type === "content_block_stop") { assert.equal(open, true); assert.equal(event.index, index++); open = false; }
		else assert.fail(`unexpected event ${event.type}`);
	}
	assert.equal(open, false);
}

function syntheticPrefix(): Event[] {
	return [
		{ type: "message_start", message: { id: "msg_synthetic", type: "message", role: "assistant", model: "backend-model", content: [], stop_reason: null, stop_sequence: null, usage: { input_tokens: 1, output_tokens: 0 } } },
		{ type: "content_block_start", index: 0, content_block: { type: "text", text: "" } },
		{ type: "content_block_delta", index: 0, delta: { type: "text_delta", text: "partial" } },
	];
}
function sendEvent(response: ServerResponse, event: Event) { response.write(`event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`); }
function sendLines(response: ServerResponse, input: string) {
	response.setHeader("Content-Type", "text/event-stream");
	for (const line of input.split(/\r?\n/).filter((line) => line.trim() !== "")) {
		response.write(`event: ${JSON.parse(line).type}\ndata: ${line}\n\n`);
	}
	response.end();
}

class ReplayServer {
	url = "";
	requests: Event[] = [];
	violations: string[] = [];
	private constructor(private readonly reply: (response: ServerResponse, body: Event, index: number) => void | Promise<void>) {}
	private server = createServer(async (request, response) => {
		try {
			const chunks: Buffer[] = [];
			for await (const chunk of request) chunks.push(Buffer.from(chunk));
			const body = JSON.parse(Buffer.concat(chunks).toString());
			if (request.url !== "/v1/messages") this.violations.push("path");
			if (request.headers["x-api-key"] !== UPSTREAM_KEY) this.violations.push("upstream-auth");
			if (request.headers.authorization || request.headers["x-access-token"]) this.violations.push("forwarded-caller-credential");
			const index = this.requests.length;
			this.requests.push(body);
			await this.reply(response, body, index);
		} catch (error) { response.destroy(error instanceof Error ? error : new Error(String(error))); }
	});
	static async start(reply: (response: ServerResponse, body: Event, index: number) => void | Promise<void>): Promise<ReplayServer> {
		const server = new ReplayServer(reply);
		await new Promise<void>((resolveListen) => server.server.listen(0, "127.0.0.1", resolveListen));
		const address = server.server.address();
		assert.ok(address && typeof address !== "string");
		server.url = `http://127.0.0.1:${address.port}`;
		return server;
	}
	async stop() { this.server.closeAllConnections(); await new Promise<void>((resolveClose) => this.server.close(() => resolveClose())); }
}
