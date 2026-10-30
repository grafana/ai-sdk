import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { chmodSync, existsSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, relative, resolve } from "node:path";
import { describe, it } from "node:test";
import { parseAllDocuments, parseDocument } from "yaml";

const root = resolve(import.meta.dirname, "../../..");
const document = parseDocument(readFileSync(resolve(root, ".github/workflows/ci.yml"), "utf8"));
assert.deepEqual(document.errors, []);
const workflow = document.toJS() as { concurrency: { group: string; "cancel-in-progress": string }; on: { push: { branches: string[]; tags?: string[] }; pull_request: unknown }; jobs: Record<string, { name?: string; if?: string; needs?: string | string[]; permissions?: Record<string, string>; concurrency?: unknown; secrets?: unknown; uses?: string; with?: Record<string, unknown>; "continue-on-error"?: boolean; steps?: Array<{ name?: string; if?: string; run?: string; uses?: string; env?: Record<string, string>; with?: Record<string, unknown>; "continue-on-error"?: boolean }> }> };
const jobs = workflow.jobs;
const source = ["ci", "race", "docs-lint", "module-resolution", "parity-baseline", "integration-test", "conformance-test"];
const pushGuard = "${{ github.event_name == 'push' && (github.ref == 'refs/heads/main' || startsWith(github.ref, 'refs/tags/ai-gateway/v')) && github.repository == 'grafana/ai-sdk' }}";
const publishFile = "publish-ai-gateway-dev.yml";
const sharedCD = /^grafana\/plugin-ci-workflows\/\.github\/workflows\/cd\.yml@ci-cd-workflows\/v\d+\.\d+\.\d+$/;
const publishDocument = parseDocument(readFileSync(resolve(root, ".github/workflows", publishFile), "utf8"));
assert.deepEqual(publishDocument.errors, []);
const publishing = publishDocument.toJS() as { on: unknown; permissions: unknown; concurrency: unknown; jobs: typeof jobs };
const normalize = (value: string | undefined) => value?.replace(/\s+/g, " ").trim();
const needs = (job: string) => [jobs[job]!.needs ?? []].flat();
const steps = (job: string) => jobs[job]!.steps ?? [];
const commands = (job: string) => steps(job).map((step) => step.run ?? "").join("\n");
const dependencies = (job: string) => {
  const seen = new Set<string>();
  const pending = [...needs(job)];
  while (pending.length > 0) {
    const dependency = pending.pop()!;
    assert.ok(jobs[dependency], `missing dependency ${dependency}`);
    if (seen.has(dependency)) continue;
    seen.add(dependency);
    pending.push(...needs(dependency));
  }
  return seen;
};

describe("candidate-source workflow gates", () => {
  it("keeps pnpm packages under the shared lockfile root", () => {
    const workspace = parseDocument(readFileSync(resolve(root, "pnpm-workspace.yaml"), "utf8")).toJS() as { packages: string[] };
    const lockfile = parseAllDocuments(readFileSync(resolve(root, "pnpm-lock.yaml"), "utf8")).at(-1)?.toJS() as { importers: Record<string, unknown> };
    for (const packagePath of workspace.packages) {
      assert.equal(relative(root, resolve(root, packagePath)), packagePath);
      assert.ok(Object.hasOwn(lockfile.importers, packagePath), `${packagePath} missing from root lockfile`);
    }
  });

  it("keeps candidate-source checks blocking without standalone prerequisites", () => {
    for (const job of source) {
      assert.ok(jobs[job], `missing source check ${job}`);
      assert.equal(jobs[job]!.if, job === "ci" ? "${{ always() }}" : undefined, `${job} must run on PR and push`);
      const required = dependencies(job);
      assert.ok(!required.has("plugin-validation"), `${job} depends on the artifact gate`);
      for (const path of [job, ...required]) {
        assert.doesNotMatch(commands(path), /test-candidate-source|test-go-client-standalone/);
      }
    }
    for (const task of ["test-module-policy", "test-ci-workflow", "verify-ai-gateway-boundary", "verify-module-requirements", "verify-sdk-gateway-isolation"]) {
      assert.match(commands("module-resolution"), new RegExp(`mise run ${task}`));
    }
    assert.match(commands("parity-baseline"), /test-providerwire-v4/);
    assert.match(commands("conformance-test"), /test-conformance/);
    assert.match(commands("integration-test"), /test-integration/);
    for (const task of ["fmt-check", "release-check", "vet", "build", "test-short"]) {
      assert.match(commands("build-test"), new RegExp(`mise run ${task}\\b`));
    }
    assert.match(commands("lint"), /mise run lint/);
    assert.match(commands("race"), /mise run test-race/);
  });

  it("validates the plugin package on pull requests and keeps it only for eligible pushes", () => {
    const job = jobs["plugin-validation"];
    assert.ok(job, "missing plugin validation job");
    assert.equal(job.if, undefined, "plugin validation must run on PR and push");
    const jobSteps = steps("plugin-validation");
    assert.ok(jobSteps.some((step) => step.uses?.startsWith("actions/checkout@") && step.with?.["persist-credentials"] === false));
    assert.ok(jobSteps.every((step) => step["continue-on-error"] !== true));
    const clean = jobSteps.find((step) => step.name === "Verify clean plugin source at the artifact revision")!;
    assert.equal(normalize(clean.if), "${{ github.event_name == 'push' }}");
    assert.match(clean.run!, /git rev-parse HEAD.*GITHUB_SHA/);
    assert.match(clean.run!, /git status --porcelain --untracked-files=all/);
    const order = ["generate:kinds", "mise run test-ai-gateway-frontend", "mise run build-ai-gateway-plugin", "mise run validate-ai-gateway-plugin", "mise run test-ai-gateway-e2e", "mise run test-ai-gateway\n", "mise run test-ai-gateway-command"];
    const script = `${commands("plugin-validation")}\n`;
    let position = -1;
    for (const command of order) {
      const next = script.indexOf(command, position + 1);
      assert.ok(next > position, `${command} must run in order`);
      position = next;
    }
    assert.match(commands("plugin-validation"), /git status --porcelain --untracked-files=all -- ai-gateway\)"/);
    const e2e = jobSteps.find((step) => step.run === "mise run test-ai-gateway-e2e")!;
    assert.match(e2e.env?.GRAFANA_VERSION ?? "", /^nightly@sha256:[0-9a-f]{64}$/);
    const build = jobSteps.find((step) => step.run === "mise run build-ai-gateway-plugin")!;
    assert.equal(build.env, undefined, "the build derives its source revision from the checkout");
    const caches = jobSteps.filter((step) => step.uses?.startsWith("actions/cache@"));
    assert.equal(caches.length, 1, "plugin validation caches pnpm directly");
    const goCache = jobSteps.find((step) => step.uses === "./.github/actions/cache-go")!;
    assert.ok(goCache, "plugin validation uses isolated module and build caches");
    assert.equal(normalize(goCache.with?.["lookup-only"] as string), "${{ startsWith(github.ref, 'refs/tags/') }}");
    for (const cache of caches) assert.equal(normalize(cache.with?.["lookup-only"] as string), "${{ startsWith(github.ref, 'refs/tags/') }}");
    const keep = jobSteps.at(-1)!;
    assert.match(keep.uses!, /^actions\/upload-artifact@[0-9a-f]{40}$/);
    assert.equal(normalize(keep.if), pushGuard);
    assert.equal(keep.with?.path, "ai-gateway/dist");
    assert.equal(keep.with?.["if-no-files-found"], "error");
  });

  it("has no image, publication or deployment jobs", () => {
    for (const job of ["image-validation", "publish-ai-gateway-image", "publish-ai-gateway-dev", "deploy-ai-gateway"]) assert.equal(jobs[job], undefined, job);
    for (const job of Object.keys(jobs)) {
      assert.doesNotMatch(commands(job), /docker|Dockerfile|ghcr\.io/i, job);
      for (const step of steps(job)) assert.doesNotMatch(step.uses ?? "", /docker\/|trigger-argo-workflow/, job);
    }
    assert.doesNotMatch(readFileSync(resolve(root, "mise.toml"), "utf8"), /Dockerfile|build-ai-gateway-image|test-ai-gateway-image-source/);
  });

  it("publishes to the dev catalog only after source CI for canonical main pushes", () => {
    assert.deepEqual(publishing.on, { push: { branches: ["main"] } });
    assert.deepEqual(publishing.permissions, {});
    assert.deepEqual(publishing.concurrency, { group: "ai-gateway-dev-publish", "cancel-in-progress": false });
    assert.deepEqual(Object.keys(publishing.jobs).sort(), ["publish", "source-ci", "update-plugin-version"]);

    const gate = publishing.jobs["source-ci"]!;
    assert.equal(gate.if, "${{ github.repository == 'grafana/ai-sdk' }}");
    assert.deepEqual(gate.permissions, { contents: "read", actions: "read", checks: "read" });
    assert.ok(gate.steps!.some((step) => step.uses?.startsWith("actions/checkout@") && step.with?.["persist-credentials"] === false));
    assert.ok(gate.steps!.every((step) => step["continue-on-error"] !== true));
    const wait = gate.steps!.at(-1)!;
    assert.equal(wait.run, "node release/tooling/gateway-publication-gate.mjs");
    assert.deepEqual(wait.env, { GH_READ_TOKEN: "${{ github.token }}" });
    assert.equal(jobs["plugin-validation"]!.name, "Validate AI Gateway plugin", "the gate requires plugin validation by its job name");

    const job = publishing.jobs.publish!;
    assert.deepEqual(job.needs, "source-ci");
    assert.equal(job.if, undefined, "publication must not run when the gate fails or is skipped");
    assert.match(job.uses ?? "", sharedCD, "pin the shared workflow to a release tag");
    assert.equal(job.steps, undefined);
    assert.equal(job.secrets, undefined, "the shared workflow reads its credentials through OIDC");
    assert.notEqual(job["continue-on-error"], true);
    assert.deepEqual(job.permissions, { contents: "write", "id-token": "write", attestations: "write", "pull-requests": "read" });
    const lintVersion = /^golangci-lint = "([^"]+)"$/m.exec(readFileSync(resolve(root, "mise.toml"), "utf8"))?.[1];
    assert.ok(lintVersion, "missing golangci-lint pin");
    assert.deepEqual(job.with, {
      branch: "${{ github.sha }}",
      environment: "dev",
      scopes: "grafana_cloud",
      "plugin-directory": "ai-gateway",
      "plugin-version-suffix": "${{ github.sha }}",
      "backend-build-target": "buildPlugin",
      "golangci-lint-version": lintVersion,
      "run-plugin-validator": true,
      "run-playwright": false,
      "trigger-argo": false,
    });
    assert.match(readFileSync(resolve(root, "ai-gateway/Magefile.go"), "utf8"), /^func BuildPlugin\(\) error \{$/m);
    const packageManager = (path: string) => (JSON.parse(readFileSync(resolve(root, path), "utf8")) as { packageManager?: string }).packageManager;
    assert.ok(packageManager("package.json"), "missing root packageManager");
    assert.equal(packageManager("ai-gateway/package.json"), packageManager("package.json"), "the shared workflow installs pnpm from the plugin package.json");

    for (const name of Object.keys(jobs)) {
      assert.equal(jobs[name]!.uses, undefined, `${name}: publication must not lengthen or cancel the required CI run`);
    }
    const workflows = resolve(root, ".github/workflows");
    for (const file of readdirSync(workflows).filter((name) => name !== publishFile)) {
      assert.doesNotMatch(readFileSync(resolve(workflows, file), "utf8"), /plugin-ci-workflows/, `${file} must not publish the plugin`);
    }
  });

  it("pins exactly the published version in dev after a successful publication", () => {
    const job = publishing.jobs["update-plugin-version"]!;
    assert.equal(job.needs, "publish");
    assert.equal(job.if, "${{ github.repository == 'grafana/ai-sdk' }}", "no status function, so a failed or skipped publish skips the pin");
    assert.deepEqual(job.permissions, { contents: "read", "id-token": "write" });
    assert.equal(job.uses, undefined);
    assert.equal(job.secrets, undefined);
    const pinSteps = job.steps!;
    assert.ok(pinSteps.every((step) => step["continue-on-error"] !== true && step.if === undefined));
    assert.deepEqual(pinSteps.map((step) => step.uses?.replace(/@[0-9a-f]{40}$/, "")), [
      "grafana/shared-workflows/actions/login-to-gar",
      "grafana/shared-workflows/actions/create-github-app-token",
      undefined,
    ], "pin shared actions by commit SHA");
    assert.deepEqual(pinSteps[1]!.with, { github_app: "updater-for-ci", permission_set: "deploy" });
    const pin = pinSteps[2]!;
    assert.deepEqual(pin.env, {
      GITHUB_TOKEN: "${{ steps.generate-token.outputs.token }}",
      PLUGIN_VERSION: "${{ fromJSON(needs.publish.outputs.plugin).version }}",
    });
    assert.doesNotMatch(pin.run!, /\$\{\{/, "pass expressions through env");

    const sha = "0123456789abcdef0123456789abcdef01234567";
    const run = (version: string) => {
      const stub = mkdtempSync(join(tmpdir(), "pin-"));
      try {
        writeFileSync(join(stub, "docker"), `#!/usr/bin/env bash\nprintf '%s\\0' "$@" > "${stub}/args"\nprintf '%s' "$GITHUB_TOKEN" > "${stub}/token"\n`);
        chmodSync(join(stub, "docker"), 0o755);
        const result = spawnSync("bash", ["-c", pin.run!], {
          env: {
            ...process.env,
            PATH: `${stub}:${process.env.PATH}`,
            GITHUB_TOKEN: "test-token",
            PLUGIN_VERSION: version,
            GITHUB_SHA: sha,
            GITHUB_SERVER_URL: "https://github.com",
            GITHUB_REPOSITORY: "grafana/ai-sdk",
            GITHUB_RUN_ID: "42",
          },
        });
        const called = existsSync(join(stub, "args"));
        return {
          status: result.status,
          args: called ? readFileSync(join(stub, "args"), "utf8").split("\0").slice(0, -1) : undefined,
          token: called ? readFileSync(join(stub, "token"), "utf8") : undefined,
        };
      } finally {
        rmSync(stub, { recursive: true, force: true });
      }
    };

    const version = `0.1.0-alpha.1+${sha}`;
    const published = run(version);
    assert.equal(published.status, 0);
    assert.equal(published.token, "test-token");
    const config = published.args!.find((arg) => arg.startsWith("CONFIG_JSON="));
    assert.deepEqual(published.args, ["run", "--rm", "-e", "GITHUB_TOKEN", "-e", config, "us-docker.pkg.dev/grafanalabs-global/docker-deployment-tools-prod/updater"]);
    assert.deepEqual(JSON.parse(config!.slice("CONFIG_JSON=".length)), {
      pull_request_branch_prefix: "auto-merge/updater/grafana-aigateway-app/dev",
      pull_request_enabled: true,
      pull_request_existing_strategy: "replace",
      pull_request_message: "Created by https://github.com/grafana/ai-sdk/actions/runs/42. This PR will auto-merge once CI has passed.",
      update_jsonnet_attribute_configs: [{
        file_path: "ksonnet/environments/appplugins/grafana-aigateway-app/versions.libsonnet",
        jsonnet_key: "dev",
        jsonnet_value: version,
      }],
    });

    for (const mismatch of ["", `+${sha}`, "0.1.0-alpha.1", `0.1.0-alpha.1+${"f".repeat(40)}`, `${version}-dirty`]) {
      const refused = run(mismatch);
      assert.notEqual(refused.status, 0, mismatch);
      assert.equal(refused.args, undefined, `${mismatch}: must not request a pin`);
    }
  });

  it("keeps the required source checks and workers without image automation", () => {
    assert.deepEqual(Object.keys(jobs).sort(), [...source, "build-test", "lint", "plugin-validation"].sort());
    assert.deepEqual(workflow.on.push, { branches: ["main"], tags: ["ai-gateway/v*"] });
    assert.ok(Object.hasOwn(workflow.on, "pull_request"));
    assert.deepEqual(Object.keys(workflow.on).sort(), ["pull_request", "push"]);
    for (const job of Object.keys(jobs)) {
      dependencies(job);
      assert.notEqual(jobs[job]!.permissions?.packages, "write");
      assert.doesNotMatch(commands(job), /docker (build|buildx|run|login|push)|build-ai-gateway-image|test-ai-gateway-image-source/);
      assert.ok(steps(job).every((step) => !step.uses?.startsWith("docker/")));
    }
  });

  it("cancels superseded PR runs while keeping main runs independent", () => {
    assert.deepEqual(workflow.concurrency, {
      group: "${{ github.workflow }}-${{ github.event.pull_request.number || github.run_id }}",
      "cancel-in-progress": "${{ github.event_name == 'pull_request' }}",
    });
  });

  it("fails the ci gate unless both independent workers succeed", () => {
    assert.deepEqual(needs("ci"), ["build-test", "lint"]);
    assert.deepEqual(needs("build-test"), []);
    assert.deepEqual(needs("lint"), []);
    for (const worker of ["build-test", "lint"]) {
      assert.equal(jobs[worker]!.if, undefined);
      assert.notEqual(jobs[worker]!["continue-on-error"], true);
      assert.ok(steps(worker).every((step) => step["continue-on-error"] !== true));
    }
    const gate = steps("ci")[0] as { env: Record<string, string> };
    assert.deepEqual(gate.env, {
      BUILD_TEST_RESULT: "${{ needs.build-test.result }}",
      LINT_RESULT: "${{ needs.lint.result }}",
    });
    for (const build of ["success", "failure", "cancelled", "skipped"]) {
      for (const lint of ["success", "failure", "cancelled", "skipped"]) {
        const result = spawnSync("bash", ["-c", commands("ci")], {
          env: { ...process.env, BUILD_TEST_RESULT: build, LINT_RESULT: lint },
        });
        assert.equal(result.status === 0, build === "success" && lint === "success", `${build}/${lint}`);
      }
    }
  });

  it("refreshes isolated build caches for every Go job", () => {
    const cacheDocument = parseDocument(readFileSync(resolve(root, ".github/actions/cache-go/action.yml"), "utf8"));
    assert.deepEqual(cacheDocument.errors, []);
    const cache = cacheDocument.toJS() as { inputs: { "lookup-only": { default: string } }; runs: { using: string; steps: Array<{ uses?: string; with?: { key: string; path: string; "restore-keys": string; "lookup-only": string } }> } };
    assert.equal(cache.runs.using, "composite");
    assert.equal(cache.inputs["lookup-only"].default, "false");
    const caches = cache.runs.steps.filter((step) => step.uses?.startsWith("actions/cache@"));
    assert.equal(caches.length, 2);
    assert.notEqual(caches[0]!.with!.path, caches[1]!.with!.path);
    for (const step of caches) {
      assert.equal(step.with!["lookup-only"], "${{ inputs.lookup-only }}");
      assert.ok(step.with!.key.includes("${{ runner.arch }}"));
      assert.ok(step.with!.key.includes("${{ github.job }}"));
      assert.ok(step.with!.key.includes("${{ steps.paths.outputs.version }}"));
    }
    assert.ok(!caches[0]!.with!.key.includes("${{ github.sha }}"));
    assert.ok(caches[1]!.with!.key.endsWith("-${{ github.sha }}"));
    assert.ok(!caches[1]!.with!["restore-keys"].includes("${{ github.sha }}"));
    for (const job of ["build-test", "lint", "race", "module-resolution", "parity-baseline", "integration-test", "conformance-test"]) {
      const jobSteps = steps(job);
      const cacheIndex = jobSteps.findIndex((step) => step.uses === "./.github/actions/cache-go");
      assert.ok(cacheIndex > jobSteps.findIndex((step) => step.uses?.startsWith("actions/checkout@")), job);
      assert.ok(cacheIndex < jobSteps.findIndex((step) => step.run), job);
    }
  });
});
