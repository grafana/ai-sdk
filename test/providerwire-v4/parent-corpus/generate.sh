#!/usr/bin/env bash
set -euo pipefail

parent_commit=32e5ab7f1ab9e524477cc0ece04c690a89854a24
repo_root=$(git rev-parse --show-toplevel)
output=${1:-"$repo_root/gateway/providerwire/testdata/parent_request_compat_v1.json"}
temp_dir=$(mktemp -d)
cleanup() {
  git -C "$repo_root" worktree remove --force "$temp_dir" >/dev/null 2>&1 || true
  rm -rf "$temp_dir"
}
trap cleanup EXIT

git -C "$repo_root" worktree add --detach "$temp_dir" "$parent_commit" >/dev/null
cp "$repo_root/test/providerwire-v4/parent-corpus/generator.go.txt" "$temp_dir/parent_corpus_generator.go"
mkdir -p "$(dirname "$output")"
(
  cd "$temp_dir"
  go run ./parent_corpus_generator.go "$output"
)
