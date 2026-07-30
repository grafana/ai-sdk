package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/output"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/grafana/ai-sdk/schema"
)

// AlertTriage is the validated application value produced from an alert.
type AlertTriage struct {
	Severity    string   `json:"severity" jsonschema:"enum=critical,enum=warning,enum=info,description=Overall severity"`
	Category    string   `json:"category" jsonschema:"enum=infrastructure,enum=application,enum=security,enum=network"`
	RootCause   string   `json:"rootCause" jsonschema:"minLength=1,description=One-sentence likely root cause"`
	Runbook     string   `json:"runbook" jsonschema:"minLength=1,description=Concrete next step for the on-call engineer"`
	RelatedSvcs []string `json:"relatedSvcs" jsonschema:"minItems=1,description=At least one service likely affected"`
}

const alertText = `FIRING: HighErrorRate on payments-api
Error rate exceeded 5% threshold (current: 12.3%).
Labels: service=payments-api, env=production, region=us-east-1
Recent deploy: payments-api v2.4.1 rolled out 8 minutes ago.
Downstream: checkout-web reporting elevated 502s.`

func extractTriage(ctx context.Context, model provider.LanguageModel, alert string) (AlertTriage, error) {
	triageSchema, err := schema.SchemaFor[AlertTriage]()
	if err != nil {
		return AlertTriage{}, fmt.Errorf("creating triage schema: %w", err)
	}

	triageOutput, err := output.Object[AlertTriage](triageSchema)
	if err != nil {
		return AlertTriage{}, fmt.Errorf("creating triage output: %w", err)
	}

	result, err := output.GenerateObject[AlertTriage](ctx, model, triageOutput,
		aisdk.WithSystem("You are an SRE assistant. Populate every field with a concrete triage, including a likely root cause, an actionable runbook step, and at least one affected service."),
		aisdk.WithModelMessages(provider.UserText(alert)),
	)
	if err != nil {
		return AlertTriage{}, fmt.Errorf("generating triage: %w", err)
	}

	triage, err := result.Object()
	if err != nil {
		return AlertTriage{}, fmt.Errorf("validating triage: %w", err)
	}
	return triage, nil
}

func writeTriage(w io.Writer, triage AlertTriage) error {
	_, err := fmt.Fprintf(w, "Severity : %s\nCategory : %s\nCause    : %s\nRunbook  : %s\nAffected : %v\n",
		triage.Severity,
		triage.Category,
		triage.RootCause,
		triage.Runbook,
		triage.RelatedSvcs,
	)
	if err != nil {
		return fmt.Errorf("rendering triage: %w", err)
	}
	return nil
}

func main() {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY is required")
	}

	model := anthropic.New(apiKey, "claude-sonnet-5")
	triage, err := extractTriage(context.Background(), model, alertText)
	if err != nil {
		log.Fatal(err)
	}
	if err := writeTriage(os.Stdout, triage); err != nil {
		log.Fatal(err)
	}
}
