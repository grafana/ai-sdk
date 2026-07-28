// Command tools-agent shows the multi-step agent loop: the model is given tools,
// decides which to call, the SDK runs them locally and feeds the results back,
// and the loop repeats until the model produces a final answer.
//
// Ask a question that needs more than one tool, e.g.:
//
//	ANTHROPIC_API_KEY=sk-... go run . "What's the weather in Paris in Fahrenheit, and what is that plus 10?"
//
// The model will typically call get_weather, then add, then answer — you'll see
// each step printed by the OnStepFinish callback.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/anthropic"
)

// --- Tool 1: a fake weather lookup -----------------------------------------

type WeatherInput struct {
	City string `json:"city" jsonschema:"description=City to look up the weather for"`
}

type WeatherOutput struct {
	City       string `json:"city"`
	Celsius    int    `json:"celsius"`
	Fahrenheit int    `json:"fahrenheit"`
	Conditions string `json:"conditions"`
}

func getWeather(ctx context.Context, in WeatherInput, _ aisdk.ToolExecutionOptions) (WeatherOutput, error) {
	// A real tool would call an API. We fake a deterministic result.
	c := 18
	return WeatherOutput{
		City:       in.City,
		Celsius:    c,
		Fahrenheit: c*9/5 + 32,
		Conditions: "partly cloudy",
	}, nil
}

// --- Tool 2: a calculator (so the model must chain calls) -------------------

type AddInput struct {
	A float64 `json:"a" jsonschema:"description=First addend"`
	B float64 `json:"b" jsonschema:"description=Second addend"`
}

type AddOutput struct {
	Sum float64 `json:"sum"`
}

func add(ctx context.Context, in AddInput, _ aisdk.ToolExecutionOptions) (AddOutput, error) {
	return AddOutput{Sum: in.A + in.B}, nil
}

func main() {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY is required")
	}

	question := "What is the weather in Paris in Fahrenheit, and what is that number plus 10?"
	if len(os.Args) > 1 {
		question = strings.Join(os.Args[1:], " ")
	}

	model := anthropic.New(apiKey, "claude-sonnet-5")

	// TypedTool derives the JSON schema from the Go input type and handles
	// marshal/unmarshal, so Execute works with real Go structs.
	weatherTool, err := aisdk.TypedTool(aisdk.TypedToolDef[WeatherInput, WeatherOutput]{
		Name:        "get_weather",
		Description: "Get the current weather for a city, in both Celsius and Fahrenheit.",
		Execute:     getWeather,
	})
	if err != nil {
		log.Fatal(err)
	}

	addTool, err := aisdk.TypedTool(aisdk.TypedToolDef[AddInput, AddOutput]{
		Name:        "add",
		Description: "Add two numbers.",
		Execute:     add,
	})
	if err != nil {
		log.Fatal(err)
	}

	result := aisdk.StreamText(context.Background(), model,
		aisdk.WithSystem("You are a helpful assistant. Use the provided tools to answer; do not guess numbers."),
		aisdk.WithModelMessages(provider.UserText(question)),
		aisdk.WithTools(aisdk.ToolSet{
			"get_weather": weatherTool,
			"add":         addTool,
		}),
		// Without this, the loop runs a single step and stops at the first tool
		// call. Allowing several steps is what makes it an agent.
		aisdk.WithStopWhen(aisdk.StepCountIs(5)),
		// Observe the loop: one callback per completed step.
		aisdk.OnStepFinish(func(s aisdk.OnStepFinishState) {
			for _, tc := range s.ToolCalls {
				fmt.Printf("  step %d → called %s(%s)\n", s.StepNumber, tc.ToolName, string(tc.Input))
			}
		}),
	)

	// Block until the loop finishes, then print the final answer.
	result.Wait()
	if err := result.Err(); err != nil {
		log.Fatalf("error: %v", err)
	}

	fmt.Printf("\nanswer: %s\n", result.Text())
	fmt.Printf("(completed in %d step(s))\n", len(result.Steps()))
}
