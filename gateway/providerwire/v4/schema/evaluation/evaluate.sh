#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/../../../.." && pwd)
input_dir="$root/providerwire/v4/schema/evaluation"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

for run in a b; do
  mkdir -p "$tmp/go-jsonschema-$run" "$tmp/oapi-codegen-$run"
  GOWORK=off go run github.com/atombender/go-jsonschema@v0.24.1 \
    --package generated --struct-name-from-title \
    --output "$tmp/go-jsonschema-$run/types.go" \
    "$input_dir/difficult-unions.json"
  GOWORK=off go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 \
    -generate types,skip-prune -package generated \
    -o "$tmp/oapi-codegen-$run/types.go" \
    "$input_dir/difficult-unions-openapi.yaml"
done

cmp "$tmp/go-jsonschema-a/types.go" "$tmp/go-jsonschema-b/types.go"
cmp "$tmp/oapi-codegen-a/types.go" "$tmp/oapi-codegen-b/types.go"

printf 'module generated\n\ngo 1.26.3\n' > "$tmp/go-jsonschema-a/go.mod"
cat > "$tmp/go-jsonschema-a/evaluation_test.go" <<'EOF'
package generated

import (
	"encoding/json"
	"testing"
)

func TestObservedSemantics(t *testing.T) {
	raw := []byte(`{"message":{"role":"system","content":"x","inactive":true},"tool":{"type":"function","name":"f","strict":false,"inactive":true},"file":{"type":"data","data":"eA==","url":"inactive"},"optionalFalse":false,"explicitEmpty":[],"nullableOpaque":null,"nonNullOpaque":null,"keyedObjects":{"provider":{"value":1}}}`)
	var value GeneratorDifficultUnionCorpus
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if value.OptionalFalse == nil || *value.OptionalFalse {
		t.Fatal("optional false presence was not preserved")
	}
	if value.NonNullOpaque != nil {
		t.Fatal("expected generated non-null opaque field to accept null")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	if _, present := object["explicitEmpty"]; present {
		t.Fatal("expected generated empty collection to collapse to absence")
	}
	file := object["file"].(map[string]any)
	if file["url"] != "inactive" {
		t.Fatal("expected inactive union sibling to remain accepted")
	}
	if object["keyedObjects"].(map[string]any)["provider"] == nil {
		t.Fatal("keyed object map was not preserved")
	}
}
EOF
(cd "$tmp/go-jsonschema-a" && GOWORK=off go test ./...)

printf 'module generated\n\ngo 1.26.3\n\nrequire github.com/oapi-codegen/runtime v1.7.0\n' > "$tmp/oapi-codegen-a/go.mod"
cat > "$tmp/oapi-codegen-a/evaluation_test.go" <<'EOF'
package generated

import (
	"encoding/json"
	"testing"
)

func TestObservedSemantics(t *testing.T) {
	raw := []byte(`{"message":{"role":"system","content":"x","inactive":true},"tool":{"type":"function","name":"f","strict":false,"inactive":true},"file":{"type":"data","data":"eA==","url":"inactive"},"optionalFalse":false,"explicitEmpty":[],"nullableOpaque":null,"nonNullOpaque":null,"keyedObjects":{"provider":{"value":1}}}`)
	var value GeneratorDifficultUnionCorpus
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if value.OptionalFalse == nil || *value.OptionalFalse {
		t.Fatal("optional false presence was not preserved")
	}
	if value.NonNullOpaque != nil {
		t.Fatal("expected generated non-null opaque field to accept null")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	explicitEmpty, present := object["explicitEmpty"].([]any)
	if !present || len(explicitEmpty) != 0 {
		t.Fatal("expected pointer collection to preserve explicit empty")
	}
	file := object["file"].(map[string]any)
	if file["url"] != "inactive" {
		t.Fatal("expected raw union to preserve an inactive sibling without validation")
	}
	if object["keyedObjects"].(map[string]any)["provider"] == nil {
		t.Fatal("keyed object map was not preserved")
	}
}
EOF
(cd "$tmp/oapi-codegen-a" && GOWORK=off go mod tidy && GOWORK=off go test ./...)
