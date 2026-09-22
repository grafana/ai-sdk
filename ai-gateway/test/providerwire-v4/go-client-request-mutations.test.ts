import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { cpSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { describe, it } from "node:test";

const clientSource = resolve(import.meta.dirname, "../../../providers/grafana");
const cases = [
  { name: "body headers", file: "request.go", from: 'out["headers"] = opts.Headers', to: '_ = opts.Headers' },
  { name: "scalar presence", file: "request.go", from: 'if !reflect.ValueOf(value).IsNil() {', to: 'if false && !reflect.ValueOf(value).IsNil() {' },
  { name: "reasoning default omission", file: "request.go", from: 'case provider.ReasoningProviderDefault:\n', to: 'case provider.ReasoningProviderDefault:\n\t\tout["reasoning"] = "provider-default"\n' },
  { name: "binary conversion", file: "request.go", from: 'base64.StdEncoding.EncodeToString(d.Bytes)', to: 'base64.StdEncoding.EncodeToString(append([]byte{255}, d.Bytes...))' },
  { name: "URL preservation", file: "request.go", from: 'requestObject{"type": "url", "url": d.URL}', to: 'requestObject{"type": "url", "url": "https://incorrect.invalid/"}' },
  { name: "opaque provider options", file: "request.go", from: 'out["providerOptions"] = values', to: 'out["providerOptions"] = requestObject{}' },
  { name: "call header precedence", file: "provider.go", from: 'headers.Set(canonical, value)', to: 'headers.Add(canonical, value)' },
  { name: "stream protocol flag", file: "model.go", from: 'strconv.FormatBool(streaming)', to: 'strconv.FormatBool(!streaming)' },
];

describe("Go request differential red controls", () => {
  for (const mutation of cases) it(`rejects an isolated ${mutation.name} regression`, () => {
    const directory = mkdtempSync(join(tmpdir(), "wp7-request-mutation-"));
    try {
      const source = join(directory, "client");
      cpSync(clientSource, source, { recursive: true });
      const path = join(source, mutation.file);
      const original = readFileSync(path, "utf8");
      assert.equal(original.split(mutation.from).length, 2, "mutation must target one reviewed production expression");
      writeFileSync(path, original.replace(mutation.from, mutation.to));
      const env: NodeJS.ProcessEnv = { ...process.env, GRAFANA_CLIENT_MUTATION_SOURCE: source };
      delete env.NODE_TEST_CONTEXT;
      const result = spawnSync(process.execPath, [
        "--import", import.meta.resolve("tsx"), "--test",
        "--test-name-pattern=protected header|comprehensive|requests and unary|stream order",
        resolve(import.meta.dirname, "go-client-differential.test.ts"),
      ], {
        cwd: import.meta.dirname,
        env,
        encoding: "utf8", timeout: 60_000, maxBuffer: 8 << 20,
      });
      assert.equal(result.error, undefined);
      assert.equal(result.status, 1, "the same pinned differential suite must reject the mutant");
      assert.match(result.stdout + result.stderr, /AssertionError/, "red control must fail semantically, not during build or setup");
    } finally { rmSync(directory, { recursive: true, force: true }); }
  });
});
