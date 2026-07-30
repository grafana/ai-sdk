package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/anthropic"
)

type weatherInput struct {
	City string `json:"city" jsonschema:"description=City to look up the weather for"`
}

type weatherOutput struct {
	City       string `json:"city"`
	Celsius    int    `json:"celsius"`
	Conditions string `json:"conditions"`
}

func getWeather(_ context.Context, input weatherInput, _ aisdk.ToolExecutionOptions) (weatherOutput, error) {
	return weatherOutput{
		City:       input.City,
		Celsius:    18,
		Conditions: "partly cloudy",
	}, nil
}

func newAgent(model provider.LanguageModel) (*aisdk.ToolLoopAgent, error) {
	weather, err := aisdk.TypedTool(aisdk.TypedToolDef[weatherInput, weatherOutput]{
		Name:        "get_weather",
		Description: "Return deterministic sample weather data for a city.",
		Execute:     getWeather,
	})
	if err != nil {
		return nil, err
	}

	return aisdk.NewToolLoopAgent(model,
		aisdk.WithToolLoopAgentID("weather-assistant"),
		aisdk.WithToolLoopAgentOptions(
			aisdk.WithInstructions("You are a helpful assistant. Use the weather tool when the answer depends on current weather."),
			aisdk.WithTools(aisdk.ToolSet{"get_weather": weather}),
			aisdk.WithStopWhen(aisdk.StepCountIs(5)),
		),
	), nil
}

func newChatHandler(agent aisdk.Agent) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []aisdk.UIMessage `json:"messages"`
		}
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&body); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if len(body.Messages) == 0 {
			http.Error(w, "invalid messages", http.StatusBadRequest)
			return
		}
		for _, message := range body.Messages {
			switch message.Role {
			case aisdk.RoleSystem, aisdk.RoleUser, aisdk.RoleAssistant:
			default:
				http.Error(w, "invalid messages", http.StatusBadRequest)
				return
			}
		}

		stream, err := aisdk.CreateAgentUIStream(
			r.Context(),
			agent,
			body.Messages,
			aisdk.WithUIMessageStreamReasoning(false),
		)
		if err != nil {
			http.Error(w, "invalid messages", http.StatusBadRequest)
			return
		}
		if err := aisdk.PipeAgentUIStreamToResponse(w, stream); err != nil {
			log.Printf("streaming agent response: %v", err)
		}
	})
}

func main() {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY is required")
	}

	agent, err := newAgent(anthropic.New(apiKey, "claude-sonnet-5"))
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("POST /api/chat", newChatHandler(agent))

	server := http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("listening on http://localhost:8080")
	log.Fatal(server.ListenAndServe())
}
