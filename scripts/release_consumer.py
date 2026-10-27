#!/usr/bin/env python3
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile
import zipfile

from release_readiness import PREFIX, json_stream, owner, run


VERSION = "v0.0.0-releasecheck"
SCENARIOS = {
    "providers/anthropic": "TestDoStream_APIError_ReturnsAPICallErrorWithData",
    "providers/openai": "TestDoGenerate_APIError",
    "providers/openai-compatible": "TestDoGenerateSendsCompatibleRequest",
    "providers/azure": "TestNewAnthropic_ProtocolAndIdentity",
    "providers/bedrock": "TestModel_InvalidToolDoesNotInvokeProvider",
    "providers/grafana": "TestServiceAccount_Requests",
    "middleware/logger": "TestMiddleware_GenerateSuccessLogsStartAndFinish",
    "middleware/enrichment": "TestMiddlewareIntegration_GenerateStreamNoMutationRegistryAndOrdering",
    "middleware/prometheus": "TestGenerateMetrics",
    "middleware/agentobservability": "TestHooksMiddleware_AllowPassesThrough",
}


def verify_graph(graph, path, expected, cache):
    candidate = None
    actual = {}
    for module in graph:
        if module.get("Replace"):
            raise ValueError(f"consumer selected a replacement: {module['Path']}")
        if module["Path"] == path:
            candidate = module
        elif not module.get("Main"):
            actual[module["Path"]] = module.get("Version")
    if (not candidate or candidate.get("Main") or candidate.get("Version") != VERSION
            or not Path(candidate.get("Dir", "/")).resolve().is_relative_to(cache.resolve())):
        raise ValueError("consumer did not select the archived candidate from its module cache")
    if actual != expected:
        raise ValueError(f"consumer dependency graph differs: {set(actual.items()) ^ set(expected.items())}")


def verify_consumer(root, env, graph, tooling_root=None):
    if root != "." and root not in SCENARIOS:
        raise ValueError(f"no consumer regression registered for {root}")
    repo = Path.cwd()
    harness = Path(tooling_root) if tooling_root else repo
    path = PREFIX if root == "." else f"{PREFIX}/{root}"
    expected = {m["Path"]: m["Version"] for m in graph if not m.get("Main")}
    files = run("git", "ls-files", "-z").split("\0")
    roots = {str(Path(f).parent) for f in files if f == "go.mod" or f.endswith("/go.mod")}
    with tempfile.TemporaryDirectory(prefix="release-consumer-") as directory:
        temp = Path(directory)
        proxy = temp / "proxy"
        versions = proxy / path / "@v"
        versions.mkdir(parents=True)
        source = repo / root
        (versions / f"{VERSION}.mod").write_bytes((source / "go.mod").read_bytes())
        (versions / f"{VERSION}.info").write_text(json.dumps({"Version": VERSION, "Time": "2000-01-01T00:00:00Z"}))
        with zipfile.ZipFile(versions / f"{VERSION}.zip", "w") as archive:
            for file in files:
                if file and owner(file, roots) == root and not Path(file).is_symlink():
                    relative = Path(file) if root == "." else Path(file).relative_to(root)
                    archive.write(file, f"{path}@{VERSION}/{relative.as_posix()}")
        consumer = temp / "consumer"
        consumer.mkdir()
        manifest = json.loads(run("go", "mod", "edit", "-json", cwd=source, env=env))
        requirements = {m["Path"]: m["Version"] for m in manifest.get("Require", [])}
        requirements[path] = VERSION
        (consumer / "go.mod").write_text(
            f"module releasecheck.local/consumer\n\ngo {manifest['Go']}\n\nrequire (\n"
            + "".join(f"\t{p} {v}\n" for p, v in sorted(requirements.items())) + ")\n")
        (consumer / "go.sum").write_bytes((source / "go.sum").read_bytes())
        consumer_env = dict(env, GOPROXY=f"{proxy.as_uri()},{env['GOPROXY']}",
                            GONOSUMDB=",".join(filter(None, [env.get("GONOSUMDB"), path])))
        packages = list(json_stream(run("go", "list", "-json", "./...", cwd=source, env=env)))
        imports = [p["ImportPath"] for p in packages
                   if p.get("Name") != "main" and "internal" not in p["ImportPath"].split("/")]
        (consumer / "consumer_test.go").write_text(
            'package consumer\n\nimport (\n' + "".join(f'\t_ "{p}"\n' for p in imports) + ')\n')
        download_env = dict(consumer_env, GOFLAGS="-mod=mod -modcacherw")
        run("go", "mod", "download", "all", cwd=consumer, env=download_env)
        selected = list(json_stream(run("go", "list", "-m", "-json", "all", cwd=consumer, env=consumer_env)))
        verify_graph(selected, path, expected, Path(env["GOMODCACHE"]))
        subprocess.run(["go", "test", "./..."], cwd=consumer, env=consumer_env, check=True)
        if root != ".":
            scenario = SCENARIOS[root]
            results = list(json_stream(run("go", "test", "-json", "-count=1", "-run", f"^{scenario}$", path,
                                           cwd=consumer, env=consumer_env)))
            if not any(event.get("Test") == scenario and event.get("Action") == "pass" for event in results):
                raise ValueError(f"consumer regression did not run: {scenario}")
            print(f"Consumer regression passed: {root}/{scenario}")
        if root == ".":
            scenario = (harness / "test/integration/testserver/scenario_simple_text.go").read_text()
            scenario = scenario.replace('func init() {\n\tregisterScenario("simple-text", handleSimpleText)\n}', '')
            (consumer / "consumer_test.go").unlink()
            (consumer / "scenario.go").write_text(scenario)
            (consumer / "main.go").write_text('''package main
import ("net/http/httptest"; "os")
func main() {
    response := httptest.NewRecorder()
    handleSimpleText(response, httptest.NewRequest("POST", "/", nil))
    if response.Code != 200 || response.Header().Get("x-vercel-ai-ui-message-stream") != "v1" { os.Exit(1) }
    if _, err := os.Stdout.Write(response.Body.Bytes()); err != nil { os.Exit(1) }
}
''')
            wire = subprocess.check_output(["go", "run", "."], cwd=consumer, env=consumer_env)
            subprocess.run(["pnpm", "install", "--frozen-lockfile", "--filter", "@ai-sdk/test-integration"], cwd=harness, env=env, check=True)
            subprocess.run(["node", "test/integration/release-consumer.mjs"], input=wire, cwd=harness, env=env, check=True)
    print(f"Consumer verified: {path}; archived candidate and {len(expected)} pinned dependencies")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("root")
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="release-consumer-cache-") as cache:
        env = dict(os.environ, GOWORK="off", GOFLAGS="-mod=readonly -modcacherw",
                   GOPROXY="https://proxy.golang.org", GOPRIVATE="", GONOPROXY="",
                   GONOSUMDB="", GOSUMDB="sum.golang.org", GOMODCACHE=cache)
        graph = list(json_stream(run("go", "list", "-m", "-json", "all", cwd=args.root, env=env)))
        verify_consumer(args.root, env, graph)


if __name__ == "__main__":
    main()
