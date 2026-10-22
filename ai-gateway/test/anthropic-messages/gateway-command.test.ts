import assert from "node:assert/strict";
import { execFile, execFileSync, spawn, type ChildProcess } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import { createServer as createNetServer } from "node:net";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import process from "node:process";
import { promisify } from "node:util";
import { after, before, describe, it } from "node:test";
import Anthropic from "@anthropic-ai/sdk";

const GATEWAY_ROOT = resolve(import.meta.dirname, "../..");
const COMMAND_DIR = join(GATEWAY_ROOT, "cmd/grafana-ai-gateway");
const TOKEN = accessToken();
const UPSTREAM_KEY = "integration-anthropic-key";
const execFileAsync = promisify(execFile);

let gatewayBinary: string;
let goClientBinary: string;
let buildDirectory: string;

before(() => {
	buildDirectory = mkdtempSync(join(tmpdir(), "gateway-anthropic-messages-build-"));
	gatewayBinary = join(buildDirectory, "grafana-ai-gateway");
	goClientBinary = join(buildDirectory, "anthropic-go-client");
	const env = { ...process.env, GOWORK: process.env.GATEWAY_TEST_GOWORK ?? resolve(GATEWAY_ROOT, "../go.gateway.work"), GOFLAGS: `${process.env.GOFLAGS ?? ""} -mod=readonly` };
	execFileSync("go", ["build", "-o", gatewayBinary, "."], { cwd: COMMAND_DIR, env, stdio: "pipe" });
	execFileSync("go", ["build", "-o", goClientBinary, "./test/anthropic-messages/go-client"], { cwd: GATEWAY_ROOT, env, stdio: "pipe" });
});

after(() => rmSync(buildDirectory, { recursive: true, force: true }));

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

			const tools = [{ name: "weather", description: "Get weather", input_schema: { type: "object" as const, properties: { city: { type: "string" } }, required: ["city"] } }];
			const first = await client.messages.create({ model: "assistant", max_tokens: 64, tools, messages: [{ role: "user", content: "typescript tool" }] });
			const call = first.content.find((part) => part.type === "tool_use");
			assert.ok(call && call.type === "tool_use");
			assert.deepEqual(call.input, { city: "Oslo" });
			const continued = await client.messages.create({ model: "assistant", max_tokens: 64, tools, messages: [
				{ role: "user", content: "typescript tool" },
				{ role: "assistant", content: [{ type: "tool_use", id: call.id, name: call.name, input: call.input }] },
				{ role: "user", content: [{ type: "tool_result", tool_use_id: call.id, content: "snow" }] },
			] });
			assert.equal(continued.content[0]?.type, "text");
			assert.equal(continued.content[0]?.text, "tool result received");

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
			const { stdout: output } = await execFileAsync(goClientBinary, [gateway.url, TOKEN], { encoding: "utf8", timeout: 30_000 });
			const summary = JSON.parse(output) as { unary_text: string; unary_stop_reason: string; unary_input_tokens: number; unary_output_tokens: number; tool_text: string; stream_text: string; stream_events: string[] };
			assert.equal(summary.unary_text, "synthetic answer");
			assert.equal(summary.unary_stop_reason, "end_turn");
			assert.equal(summary.unary_input_tokens, 7);
			assert.equal(summary.unary_output_tokens, 3);
			assert.equal(summary.tool_text, "tool result received");
			assert.equal(summary.stream_text, "synthetic stream");
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

class SyntheticAnthropic {
	readonly server = createServer((request, response) => void this.handle(request, response));
	readonly requests: Array<{ path: string; body: Record<string, any> }> = [];
	readonly violations: string[] = [];
	url = "";
	holdStream = false;
	private cancelled = false;

	async start(): Promise<void> {
		await new Promise<void>((resolveListen) => this.server.listen(0, "127.0.0.1", resolveListen));
		const address = this.server.address();
		if (address == null || typeof address === "string") throw new Error("synthetic provider did not bind");
		this.url = `http://127.0.0.1:${address.port}`;
	}
	static async start(): Promise<SyntheticAnthropic> { const server = new SyntheticAnthropic(); await server.start(); return server; }
	async stop(): Promise<void> { this.server.closeAllConnections(); await new Promise<void>((resolveClose) => this.server.close(() => resolveClose())); }
	async waitForCancellation(): Promise<void> {
		const deadline = Date.now() + 5000;
		while (!this.cancelled && Date.now() < deadline) await new Promise((resolveWait) => setTimeout(resolveWait, 20));
		assert.equal(this.cancelled, true, "synthetic provider request was not canceled");
	}
	private async handle(request: IncomingMessage, response: ServerResponse): Promise<void> {
		const chunks: Buffer[] = [];
		for await (const chunk of request) chunks.push(Buffer.from(chunk));
		const body = JSON.parse(Buffer.concat(chunks).toString()) as Record<string, any>;
		this.requests.push({ path: request.url ?? "", body });
		if (request.url !== "/v1/messages") this.violations.push(`path:${request.url}`);
		if (request.headers["x-api-key"] !== UPSTREAM_KEY) this.violations.push("upstream-auth");
		if (request.headers["x-access-token"] || request.headers["x-grafana-id"] || request.headers.authorization) this.violations.push("caller-credential-forwarded");
		const serialized = JSON.stringify(body);
		if (serialized.includes("provider failure")) {
			response.writeHead(503, { "Content-Type": "application/json" });
			response.end(JSON.stringify({ type: "error", error: { type: "api_error", message: "synthetic provider secret" } }));
			return;
		}
		if (body.stream === true) {
			response.writeHead(200, { "Content-Type": "text/event-stream" });
			const send = (event: Record<string, unknown>) => response.write(`event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`);
			send({ type: "message_start", message: { id: "msg_synthetic", type: "message", role: "assistant", model: "backend-model", content: [], stop_reason: null, stop_sequence: null, usage: { input_tokens: 7, output_tokens: 0 } } });
			if (this.holdStream) {
				send({ type: "content_block_start", index: 0, content_block: { type: "text", text: "" } });
				send({ type: "content_block_delta", index: 0, delta: { type: "text_delta", text: "partial" } });
				request.once("close", () => { this.cancelled = true; });
				response.once("close", () => { this.cancelled = true; });
				return;
			}
			if (Array.isArray(body.tools) && body.tools.length > 0) {
				if (serialized.includes("mixed stream tool")) {
					send({ type: "content_block_start", index: 0, content_block: { type: "text", text: "" } });
					send({ type: "content_block_delta", index: 0, delta: { type: "text_delta", text: "checking weather" } });
					send({ type: "content_block_stop", index: 0 });
					send({ type: "content_block_start", index: 1, content_block: { type: "tool_use", id: "toolu_mixed", name: "weather", input: {} } });
					send({ type: "content_block_delta", index: 1, delta: { type: "input_json_delta", partial_json: '{"city":"Oslo"}' } });
					send({ type: "content_block_stop", index: 1 });
					send({ type: "message_delta", delta: { stop_reason: "tool_use", stop_sequence: null }, usage: { output_tokens: 3 } });
					send({ type: "message_stop" });
					response.end();
					return;
				}
				send({ type: "content_block_start", index: 0, content_block: { type: "tool_use", id: "toolu_stream", name: "weather", input: {} } });
				send({ type: "content_block_delta", index: 0, delta: { type: "input_json_delta", partial_json: '{"city":"Oslo"}' } });
				send({ type: "content_block_stop", index: 0 });
				send({ type: "message_delta", delta: { stop_reason: "tool_use", stop_sequence: null }, usage: { output_tokens: 3 } });
				send({ type: "message_stop" });
				response.end();
				return;
			}
			send({ type: "content_block_start", index: 0, content_block: { type: "text", text: "" } });
			send({ type: "content_block_delta", index: 0, delta: { type: "text_delta", text: "synthetic stream" } });
			send({ type: "content_block_stop", index: 0 });
			send({ type: "message_delta", delta: { stop_reason: "end_turn", stop_sequence: null }, usage: { output_tokens: 3 } });
			send({ type: "message_stop" });
			response.end();
			return;
		}
		const continued = (body.messages as Array<{ content: Array<{ type: string }> }>).some((message) => message.content.some((part) => part.type === "tool_result"));
		const wantsTools = Array.isArray(body.tools) && body.tools.length > 0;
		const content = wantsTools && !continued
			? [{ type: "tool_use", id: "toolu_synthetic", name: "weather", input: { city: "Oslo" } }]
			: [{ type: "text", text: continued ? "tool result received" : "synthetic answer" }];
		response.writeHead(200, { "Content-Type": "application/json" });
		response.end(JSON.stringify({ id: "msg_synthetic", type: "message", role: "assistant", model: "backend-model", content, stop_reason: wantsTools && !continued ? "tool_use" : "end_turn", stop_sequence: null, usage: { input_tokens: 7, output_tokens: 3 } }));
	}
}

class Gateway {
	private constructor(readonly child: ChildProcess, readonly url: string, readonly cloudURL: string, readonly operationalURL: string, readonly directory: string, readonly stderr: { value: string }) {}
	static async start(upstreamURL: string, staticKey = false): Promise<Gateway> {
		const directory = mkdtempSync(join(tmpdir(), "gateway-anthropic-messages-"));
		const config = join(directory, "models.yaml");
		const authConfig = staticKey ? `auth:\n  type: static-key\n  staticKey:\n    identities:\n      integration:\n        keyEnv: GATEWAY_TEST_STATIC_KEY\nserver:\n  cloud:\n    enabled: false\n` : "";
		writeFileSync(config, authConfig + `providers:\n  anthropic:\n    type: anthropic\n    apiKeyEnv: GATEWAY_TEST_ANTHROPIC_KEY\n    baseURL: ${upstreamURL}\nmodels:\n  grafana/assistant:\n    name: Grafana Assistant\n    primary:\n      provider: anthropic\n      model: backend-model\n    aliases:\n      - assistant\n`);
		const port = await availablePort();
		const cloudPort = await availablePort();
		const operationalPort = await availablePort();
		const url = `http://127.0.0.1:${port}`;
		const cloudURL = `http://127.0.0.1:${cloudPort}`;
		const operationalURL = `http://127.0.0.1:${operationalPort}`;
		const child = spawn(gatewayBinary, [`--config.file=${config}`, "--deployment.mode=development", ...(staticKey ? [] : ["--auth.unsafe"]), `--server.private-listen-address=127.0.0.1:${port}`, `--server.cloud-listen-address=127.0.0.1:${cloudPort}`, `--server.operational-listen-address=127.0.0.1:${operationalPort}`, "--server.shutdown-timeout=2s"], {
			stdio: ["ignore", "ignore", "pipe"], env: { ...process.env, GATEWAY_TEST_ANTHROPIC_KEY: UPSTREAM_KEY, GATEWAY_TEST_STATIC_KEY: TOKEN },
		});
		const stderr = { value: "" };
		child.stderr?.on("data", (chunk: Buffer) => { stderr.value += chunk.toString(); });
		for (let attempt = 0; attempt < 200; attempt++) {
			if (child.exitCode !== null) throw new Error(`Gateway exited during startup: ${stderr.value}`);
			try { if ((await fetch(`${operationalURL}/ready`)).ok) return new Gateway(child, url, cloudURL, operationalURL, directory, stderr); } catch {}
			await new Promise((resolveWait) => setTimeout(resolveWait, 25));
		}
		child.kill("SIGKILL");
		throw new Error(`Gateway did not become ready: ${stderr.value}`);
	}
	async stop(): Promise<void> {
		if (this.child.exitCode == null && this.child.signalCode == null) this.child.kill("SIGTERM");
		if (this.child.exitCode == null && this.child.signalCode == null) {
			await Promise.race([
				new Promise<void>((resolveExit) => this.child.once("exit", () => resolveExit())),
				new Promise<void>((resolveTimeout) => setTimeout(resolveTimeout, 3000)),
			]);
		}
		if (this.child.exitCode == null && this.child.signalCode == null) this.child.kill("SIGKILL");
		rmSync(this.directory, { recursive: true, force: true });
	}
}

async function availablePort(): Promise<number> {
	const server = createNetServer();
	await new Promise<void>((resolveListen) => server.listen(0, "127.0.0.1", resolveListen));
	const address = server.address();
	if (address == null || typeof address === "string") throw new Error("could not reserve a local port");
	const port = address.port;
	await new Promise<void>((resolveClose) => server.close(() => resolveClose()));
	return port;
}

function accessToken(): string {
	const header = Buffer.from(JSON.stringify({ alg: "ES256", typ: "at+jwt" })).toString("base64url");
	const payload = Buffer.from(JSON.stringify({ sub: "access-policy:integration", aud: ["ai-sdk"], exp: Math.floor(Date.now() / 1000) + 3600, namespace: "stacks-123", serviceIdentity: "integration-service" })).toString("base64url");
	return `${header}.${payload}.${Buffer.alloc(64).toString("base64url")}`;
}
