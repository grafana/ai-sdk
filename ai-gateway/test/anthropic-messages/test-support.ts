import assert from "node:assert/strict";
import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import { createServer as createNetServer } from "node:net";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import process from "node:process";
import { after, before } from "node:test";

const GATEWAY_ROOT = resolve(import.meta.dirname, "../..");
const COMMAND_DIR = join(GATEWAY_ROOT, "cmd/grafana-ai-gateway");
export const TOKEN = accessToken();
export const UPSTREAM_KEY = "integration-anthropic-key";

export const binaries = { gateway: "", goClient: "" };
let buildDirectory: string;

before(() => {
	buildDirectory = mkdtempSync(join(tmpdir(), "gateway-anthropic-messages-build-"));
	binaries.gateway = join(buildDirectory, "grafana-ai-gateway");
	binaries.goClient = join(buildDirectory, "anthropic-go-client");
	const env = { ...process.env, GOWORK: process.env.GATEWAY_TEST_GOWORK ?? resolve(GATEWAY_ROOT, "../go.gateway.work"), GOFLAGS: `${process.env.GOFLAGS ?? ""} -mod=readonly` };
	execFileSync("go", ["build", "-o", binaries.gateway, "."], { cwd: COMMAND_DIR, env, stdio: "pipe" });
	execFileSync("go", ["build", "-o", binaries.goClient, "./test/anthropic-messages/go-client"], { cwd: GATEWAY_ROOT, env, stdio: "pipe" });
});

after(() => rmSync(buildDirectory, { recursive: true, force: true }));

export class SyntheticAnthropic {
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
			send({ type: "message_start", message: { id: "msg_synthetic", type: "message", role: "assistant", model: "backend-model", content: [], stop_reason: null, stop_sequence: null, usage: { input_tokens: 7, cache_read_input_tokens: 2, cache_creation_input_tokens: 1, output_tokens: 0 } } });
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
		response.end(JSON.stringify({ id: "msg_synthetic", type: "message", role: "assistant", model: "backend-model", content, stop_reason: wantsTools && !continued ? "tool_use" : "end_turn", stop_sequence: null, usage: { input_tokens: 7, cache_read_input_tokens: 2, cache_creation_input_tokens: 1, output_tokens: 3 } }));
	}
}

export class Gateway {
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
		const child = spawn(binaries.gateway, [`--config.file=${config}`, "--deployment.mode=development", ...(staticKey ? [] : ["--auth.unsafe"]), `--server.private-listen-address=127.0.0.1:${port}`, `--server.cloud-listen-address=127.0.0.1:${cloudPort}`, `--server.operational-listen-address=127.0.0.1:${operationalPort}`, "--server.shutdown-timeout=2s"], {
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
