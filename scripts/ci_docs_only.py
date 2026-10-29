import os
import re
import subprocess
import sys


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
        name = path.decode("utf-8")
        if not (name in {"README.md", "CONTRIBUTING.md"} or
                (name.startswith("docs/") and name.endswith(".md"))):
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
