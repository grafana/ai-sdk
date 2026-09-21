//go:build conformance

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/grafana/ai-sdk/providers/grafana"
	"github.com/grafana/ai-sdk/test/conformance"
)

type operation string

const (
	operationExecute operation = "execute"
	operationCompare operation = "compare"
)

type request struct {
	Operation operation                     `json:"operation"`
	Directory string                        `json:"directory"`
	BaseURL   string                        `json:"baseURL"`
	Token     string                        `json:"token"`
	ModelID   string                        `json:"modelID"`
	Result    conformance.ScenarioResult    `json:"result"`
	Requests  []conformance.RequestSnapshot `json:"requests"`
}

func run() error {
	var input request
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return err
	}
	var value any
	switch input.Operation {
	case operationCompare:
		errors, err := conformance.CompareScenario(input.Directory, input.Result, input.Requests)
		if err != nil {
			return err
		}
		value = errors
	case operationExecute:
		cfg, err := conformance.LoadConfig(input.Directory + "/config.yaml")
		if err != nil {
			return err
		}
		client, err := grafana.NewWithAccessToken(grafana.AccessTokenConfig{BaseURL: input.BaseURL, AccessToken: input.Token})
		if err != nil {
			return err
		}
		model, err := client.LanguageModel(input.ModelID)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		value, err = conformance.ExecuteScenario(ctx, cfg, model)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown operation %q", input.Operation)
	}
	return json.NewEncoder(os.Stdout).Encode(value)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
