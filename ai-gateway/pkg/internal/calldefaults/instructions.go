package calldefaults

import (
	"encoding/json"
	"errors"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
)

var errInstructionsConfig = errors.New("calldefaults: the default-instructions configuration does not decode")

type instructionsConfig struct {
	Messages []struct {
		Content string `json:"content"`
	} `json:"messages"`
}

func instructionsDefinition() extension.Definition {
	return tenanttype.DefaultInstructions().Bind(tenanttype.Implementation{
		FailurePolicy: extension.FailSkip,
		New:           newInstructions,
	})
}

func newInstructions(config json.RawMessage, _ extension.Dependencies) (any, error) {
	var decoded instructionsConfig
	if err := json.Unmarshal(config, &decoded); err != nil {
		return nil, errInstructionsConfig
	}
	messages := make([]provider.Message, 0, len(decoded.Messages))
	for _, message := range decoded.Messages {
		messages = append(messages, provider.NewSystemMessage(message.Content))
	}
	return transformer{middleware: middleware.DefaultInstructions(messages...)}, nil
}
