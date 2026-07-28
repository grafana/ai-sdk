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

func main() {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY is required")
	}

	prompt := "Explain what a goroutine is in one sentence."
	if len(os.Args) > 1 {
		prompt = strings.Join(os.Args[1:], " ")
	}

	model := anthropic.New(apiKey, "claude-sonnet-5")
	result, err := aisdk.GenerateText(context.Background(), model,
		aisdk.WithSystem("You are a concise Go expert."),
		aisdk.WithModelMessages(provider.UserText(prompt)),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result.Text)
}
