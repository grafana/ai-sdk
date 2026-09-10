# Installation

Create a Go project, install the core SDK and one model provider, then make your
first model call.

## Requirements

- Go 1.26.3 or newer
- Credentials for your chosen model provider

## Create a project

Start from an empty directory:

```bash
mkdir ai-sdk-quickstart
cd ai-sdk-quickstart
go mod init example.com/ai-sdk-quickstart
```

Install the core module and the Anthropic provider used in this walkthrough:

```bash
go get github.com/grafana/ai-sdk
go get github.com/grafana/ai-sdk/providers/anthropic
```

Providers are separate modules, so applications install only the vendor
integrations they use.

## Choose another provider

| You want to call | Setup guide |
|---|---|
| Claude through Anthropic or Vertex AI | [Anthropic](../providers/anthropic.md) |
| Models hosted on Amazon Bedrock | [Amazon Bedrock](../providers/bedrock.md) |
| OpenAI through the Responses API | [OpenAI](../providers/openai.md) |
| A Chat Completions-compatible server | [OpenAI-compatible APIs](../providers/openai-compatible.md) |

Each provider guide covers its module, authentication, and model construction.
The generation APIs used below stay the same.

## Make the first call

Save this complete program as `main.go`:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/anthropic"
)

func main() {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY is required")
	}

	model := anthropic.New(apiKey, "claude-sonnet-5")
	result, err := aisdk.GenerateText(context.Background(), model,
		aisdk.WithModelMessages(provider.UserText("Reply with: ready")),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result.Text)
}
```

Build the program, then run it with an Anthropic API key:

```bash
go build ./...
ANTHROPIC_API_KEY=sk-... go run .
```

A successful call prints a short response such as:

```text
ready
```

Keep credentials in environment variables or a secret manager. The provider
setup guides link to the authentication documentation for each service.

## Next steps

- Add system instructions and streaming → [Generate text from Go](backend-only.md)
- Connect a React chat frontend → [Full-stack chat](full-stack-chat.md)
- Compare authentication and capabilities → [Choose a provider](../providers/overview.md)

---

← [Getting started](../README.md#start-building) · [Docs index](../README.md) · [Generate text from Go →](backend-only.md)
