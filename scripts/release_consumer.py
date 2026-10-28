#!/usr/bin/env python3
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import time

from release_readiness import PREFIX, VERSION, json_stream, run


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


POLL_SECONDS = 30
TAG = re.compile(r"^(?:(?P<root>(?:providers|middleware)/[a-z0-9-]+)/)?(?P<version>v" + VERSION + r")$")


def parse_tag(tag):
    match = TAG.fullmatch(tag)
    if not match:
        raise ValueError(f"not a library release tag: {tag}")
    return match.group("root") or ".", match.group("version")


def proxy_env(cache):
    return dict(os.environ, GOWORK="off", GOFLAGS="-mod=mod -modcacherw", GOPROXY="https://proxy.golang.org",
                GOPRIVATE="", GONOPROXY="", GONOSUMDB="", GOSUMDB="sum.golang.org", GOMODCACHE=str(cache))


def unavailable(modules, env):
    missing = []
    for path, version in modules:
        result = subprocess.run(["go", "mod", "download", "-json", f"{path}@{version}"],
                                env=env, text=True, capture_output=True)
        try:
            info = json.loads(result.stdout)
        except json.JSONDecodeError:
            info = {"Error": result.stderr.strip()}
        if result.returncode or info.get("Error"):
            missing.append((path, version, info.get("Error") or result.stderr.strip()))
        elif path == modules[0][0]:
            manifest = json.loads(run("go", "mod", "edit", "-json", info["GoMod"], env=env))
            modules.extend((m["Path"], m["Version"]) for m in manifest.get("Require") or []
                           if (m["Path"] == PREFIX or m["Path"].startswith(PREFIX + "/"))
                           and (m["Path"], m["Version"]) not in modules)
    return missing


def wait_until_available(path, version, minutes, sleep=time.sleep, clock=time.monotonic):
    deadline = clock() + minutes * 60
    while True:
        with tempfile.TemporaryDirectory(prefix="release-availability-") as cache:
            missing = unavailable([(path, version)], proxy_env(Path(cache)))
        if not missing:
            return
        if clock() >= deadline:
            details = "; ".join(f"{p}@{v}: {e}" for p, v, e in missing)
            raise ValueError(f"not resolvable through the public Go proxy after {minutes} minutes: {details}")
        print(f"Waiting for the public Go proxy: {', '.join(f'{p}@{v}' for p, v, _ in missing)}", flush=True)
        sleep(POLL_SECONDS)


def verify_graph(graph, path, version, cache):
    candidate = None
    for module in graph:
        if module.get("Replace"):
            raise ValueError(f"consumer selected a replacement: {module['Path']}")
        if module["Path"] == path:
            candidate = module
    if (not candidate or candidate.get("Main") or candidate.get("Version") != version
            or not Path(candidate.get("Dir", "/")).resolve().is_relative_to(cache.resolve())):
        raise ValueError(f"consumer did not select the published {path}@{version} from its module cache")


def verify_published(root, version):
    if root != "." and root not in SCENARIOS:
        raise ValueError(f"no consumer regression registered for {root}")
    harness = Path(__file__).resolve().parent.parent
    path = PREFIX if root == "." else f"{PREFIX}/{root}"
    with tempfile.TemporaryDirectory(prefix="release-consumer-") as directory:
        temp = Path(directory)
        cache = temp / "cache"
        consumer = temp / "consumer"
        consumer.mkdir()
        env = proxy_env(cache)
        run("go", "mod", "init", "releasecheck.local/consumer", cwd=consumer, env=env)
        run("go", "get", f"{path}@{version}", cwd=consumer, env=env)
        packages = list(json_stream(run("go", "list", "-json", f"{path}/...", cwd=consumer, env=env)))
        imports = [p["ImportPath"] for p in packages if p.get("Module", {}).get("Path") == path
                   and p.get("Name") != "main" and "internal" not in p["ImportPath"].split("/")]
        (consumer / "consumer_test.go").write_text(
            'package consumer\n\nimport (\n' + "".join(f'\t_ "{p}"\n' for p in imports) + ')\n')
        run("go", "mod", "tidy", cwd=consumer, env=env)
        selected = list(json_stream(run("go", "list", "-m", "-json", "all", cwd=consumer, env=env)))
        verify_graph(selected, path, version, cache)
        subprocess.run(["go", "test", "./..."], cwd=consumer, env=env, check=True)
        if root != ".":
            scenario = SCENARIOS[root]
            results = list(json_stream(run("go", "test", "-json", "-count=1", "-run", f"^{scenario}$", path,
                                           cwd=consumer, env=env)))
            if not any(event.get("Test") == scenario and event.get("Action") == "pass" for event in results):
                raise ValueError(f"consumer regression did not run: {scenario}")
            print(f"Consumer regression passed: {root}/{scenario}")
        else:
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
            wire = subprocess.check_output(["go", "run", "."], cwd=consumer, env=env)
            subprocess.run(["pnpm", "install", "--frozen-lockfile", "--filter", "@ai-sdk/test-integration"], cwd=harness, env=env, check=True)
            subprocess.run(["node", "test/integration/release-consumer.mjs"], input=wire, cwd=harness, env=env, check=True)
    print(f"Published version verified: {path}@{version}")


def main():
    parser = argparse.ArgumentParser(description="Verify a published library release through the public Go proxy.")
    parser.add_argument("tag", help="library release tag, for example providers/openai/v0.1.0-alpha.2")
    parser.add_argument("--wait-minutes", type=int, default=0,
                        help="wait up to this long for the version and its first-party requirements to resolve")
    args = parser.parse_args()
    root, version = parse_tag(args.tag)
    if args.wait_minutes:
        wait_until_available(PREFIX if root == "." else f"{PREFIX}/{root}", version, args.wait_minutes)
    verify_published(root, version)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, subprocess.CalledProcessError) as error:
        raise SystemExit(f"release consumer: {error}")
