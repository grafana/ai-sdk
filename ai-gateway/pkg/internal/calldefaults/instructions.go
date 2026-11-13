package calldefaults

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
)

var (
	errInstructionsConfig = errors.New("calldefaults: the default-instructions configuration does not decode")
	errInstructionsCall   = errors.New("calldefaults: the call carries no default-instructions configuration")
)

// systemMessages is a namespace's decoded default-instructions
// configuration: the system messages to prepend, in order.
type systemMessages []provider.Message

type instructionsConfig struct {
	Messages []struct {
		Content string `json:"content"`
	} `json:"messages"`
}

func instructionsDefinition() extension.Definition {
	return tenanttype.DefaultInstructions().Bind(tenanttype.Implementation{
		FailurePolicy: extension.FailSkip,
		New:           func(json.RawMessage, extension.Dependencies) (any, error) { return instructionsTransformer{}, nil },
		Decode:        decodeInstructions,
	})
}

func decodeInstructions(config json.RawMessage) (any, error) {
	var decoded instructionsConfig
	if err := json.Unmarshal(config, &decoded); err != nil {
		return nil, errInstructionsConfig
	}
	messages := make(systemMessages, 0, len(decoded.Messages))
	for _, message := range decoded.Messages {
		messages = append(messages, provider.NewSystemMessage(message.Content))
	}
	return messages, nil
}

// instructionsTransformer applies each call's default instructions.
type instructionsTransformer struct{}

func (instructionsTransformer) TransformRequest(ctx context.Context, call *extension.Call, params provider.CallOptions) (provider.CallOptions, error) {
	messages, ok := call.Config.(systemMessages)
	if !ok {
		return params, errInstructionsCall
	}
	return middleware.DefaultInstructions(messages...).TransformParams(ctx, middleware.TransformParamsInput{Params: params})
}
