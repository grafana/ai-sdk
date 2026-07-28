// Command streaming-cli prints a model response as it arrives. FullStream
// provides typed events for text, reasoning, step boundaries, errors, and usage.
//
// Run it with:
//
//	ANTHROPIC_API_KEY=sk-... go run . "Explain goroutines in two sentences."
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

// deref safely reads an optional token count.
func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func main() {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY is required")
	}

	prompt := "Explain what a goroutine is in two sentences."
	if len(os.Args) > 1 {
		prompt = strings.Join(os.Args[1:], " ")
	}

	model := anthropic.New(apiKey, "claude-sonnet-5")

	result := aisdk.StreamText(context.Background(), model,
		aisdk.WithSystem("You are a concise Go expert."),
		aisdk.WithModelMessages(provider.UserText(prompt)),
	)

	for part := range result.FullStream() {
		switch p := part.(type) {
		case aisdk.StreamTextStart:
			fmt.Print("\n\033[1massistant:\033[0m ")
		case aisdk.StreamTextDelta:
			fmt.Print(p.Text)
		case aisdk.StreamReasoningDelta:
			fmt.Printf("\033[2m%s\033[0m", p.Text)
		case aisdk.StreamFinishStep:
			fmt.Println()
		case aisdk.StreamError:
			log.Fatalf("\nstream error: %v", p.Error)
		}
	}

	usage := result.TotalUsage()
	fmt.Printf("\n\033[2m[tokens: %d in / %d out · finish: %s]\033[0m\n",
		deref(usage.InputTokens.Total), deref(usage.OutputTokens.Total), result.FinishReason())

	if err := result.Err(); err != nil {
		log.Fatalf("error: %v", err)
	}
}
