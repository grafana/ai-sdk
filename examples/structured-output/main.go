// Command structured-output turns an unstructured Grafana alert into a typed,
// schema-validated Go value using output.GenerateObject.
//
// This is the SDK's answer to "I need the model to return data, not prose."
// You describe the shape with a Go struct (and jsonschema tags), the schema is
// generated from it, sent to the model to constrain the response, and the
// response is validated against it before you ever touch the value.
//
// Run it with:
//
//	ANTHROPIC_API_KEY=sk-... go run .
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/output"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/grafana/ai-sdk/schema"
)

// AlertTriage is the shape we want back. The jsonschema tags become constraints
// in the schema the model sees: enums limit the allowed values, descriptions
// guide the model, and the field set defines exactly what it must produce.
type AlertTriage struct {
	Severity    string   `json:"severity"    jsonschema:"enum=critical,enum=warning,enum=info,description=Overall severity"`
	Category    string   `json:"category"    jsonschema:"enum=infrastructure,enum=application,enum=security,enum=network"`
	RootCause   string   `json:"rootCause"   jsonschema:"description=One-sentence likely root cause"`
	Runbook     string   `json:"runbook"     jsonschema:"description=Concrete next step for the on-call engineer"`
	RelatedSvcs []string `json:"relatedSvcs" jsonschema:"description=Services likely affected"`
}

const alertText = `FIRING: HighErrorRate on payments-api
Error rate exceeded 5% threshold (current: 12.3%).
Labels: service=payments-api, env=production, region=us-east-1
Recent deploy: payments-api v2.4.1 rolled out 8 minutes ago.
Downstream: checkout-web reporting elevated 502s.`

func main() {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY is required")
	}

	model := anthropic.New(apiKey, "claude-sonnet-5")

	// 1. Generate a JSON Schema from the Go type.
	s, err := schema.SchemaFor[AlertTriage]()
	if err != nil {
		log.Fatal(err)
	}

	// 2. Wrap it in an Object output mode (Array, Choice, and JSON also exist).
	out, err := output.Object[AlertTriage](s)
	if err != nil {
		log.Fatal(err)
	}

	// 3. Generate. The result is validated against the schema before return.
	result, err := output.GenerateObject[AlertTriage](context.Background(), model, out,
		aisdk.WithSystem("You are an SRE assistant. Triage the alert into the required structure."),
		aisdk.WithModelMessages(provider.UserText(alertText)),
	)
	if err != nil {
		log.Fatal(err)
	}

	triage, err := result.Object()
	if err != nil {
		log.Fatalf("invalid structured output: %v", err)
	}

	// 4. Use it as a normal, typed Go value — no map[string]any, no manual parse.
	fmt.Printf("Severity : %s\n", triage.Severity)
	fmt.Printf("Category : %s\n", triage.Category)
	fmt.Printf("Cause    : %s\n", triage.RootCause)
	fmt.Printf("Runbook  : %s\n", triage.Runbook)
	fmt.Printf("Affected : %v\n", triage.RelatedSvcs)
}
