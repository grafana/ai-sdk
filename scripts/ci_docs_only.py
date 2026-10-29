import os
import re
import subprocess
import sys


NARRATIVE_PATHS = frozenset(
    [
        "README.md",
        "CONTRIBUTING.md",
        "docs/README.md",
        "docs/best-practices/error-handling.md",
        "docs/best-practices/production.md",
        "docs/best-practices/security.md",
        "docs/concepts/architecture.md",
        "docs/concepts/gateway-architecture.md",
        "docs/concepts/gateway-glossary.md",
        "docs/concepts/messages.md",
        "docs/concepts/providers.md",
        "docs/concepts/wire-protocol.md",
        "docs/getting-started/backend-only.md",
        "docs/getting-started/full-stack-chat.md",
        "docs/getting-started/installation.md",
        "docs/guides/agent-loops.md",
        "docs/guides/fallback-and-registry.md",
        "docs/guides/gateway-authentication.md",
        "docs/guides/retry-and-timeout.md",
        "docs/guides/streaming-http.md",
        "docs/guides/structured-output.md",
        "docs/guides/testing.md",
        "docs/guides/tool-approval.md",
        "docs/guides/tools.md",
        "docs/middleware/agent-observability.md",
        "docs/middleware/context-enrichment.md",
        "docs/middleware/overview.md",
        "docs/middleware/prometheus.md",
        "docs/middleware/structured-logging.md",
        "docs/middleware/writing-middleware.md",
        "docs/providers/anthropic.md",
        "docs/providers/azure.md",
        "docs/providers/bedrock.md",
        "docs/providers/grafana-gateway.md",
        "docs/providers/openai-compatible.md",
        "docs/providers/openai.md",
        "docs/providers/overview.md",
        "docs/providers/writing-a-provider.md",
    ]
)


def docs_only(base, head):
    if not all(re.fullmatch(r"[0-9a-f]{40}", ref or "") for ref in (base, head)):
        return False
    result = subprocess.run(
        ["git", "diff", "--raw", "-z", "--no-renames", "--no-abbrev", base, head, "--"],
        check=True, capture_output=True,
    )
    fields = result.stdout.split(b"\0")
    if fields.pop() != b"" or not fields or len(fields) % 2:
        return False
    for metadata, path in zip(fields[::2], fields[1::2]):
        parts = metadata.decode("ascii").split()
        if len(parts) != 5 or parts[4] not in {"A", "M", "D"}:
            return False
        if parts[0] not in {":100644", ":000000"} or parts[1] not in {"100644", "000000"}:
            return False
        if path.decode("utf-8") not in NARRATIVE_PATHS:
            return False
    return True


def main():
    result = False
    if os.environ.get("GITHUB_EVENT_NAME") == "pull_request":
        try:
            result = docs_only(os.environ.get("BASE_SHA"), os.environ.get("HEAD_SHA"))
        except (OSError, ValueError, subprocess.SubprocessError):
            print("Unable to classify changes; running full CI", file=sys.stderr)
    with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as output:
        print(f"docs_only={str(result).lower()}", file=output)


if __name__ == "__main__":
    main()
