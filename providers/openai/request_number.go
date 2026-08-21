package openai

import (
	"errors"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

func setResponseExtraField(body *responses.ResponseNewParams, key string, value any) {
	extra := make(map[string]any, len(body.ExtraFields())+1)
	for name, existing := range body.ExtraFields() {
		extra[name] = existing
	}
	extra[key] = value
	body.SetExtraFields(extra)
}

func clearResponseExtraField(body *responses.ResponseNewParams, key string) {
	existing := body.ExtraFields()
	if len(existing) == 0 {
		return
	}
	extra := make(map[string]any, len(existing))
	for name, value := range existing {
		if name != key {
			extra[name] = value
		}
	}
	body.SetExtraFields(extra)
}

func applyMaxOutputTokens(body *responses.ResponseNewParams, number provider.LanguageModelNumber) error {
	if integer, ok := number.Int64(); ok {
		body.MaxOutputTokens = param.NewOpt(integer)
		clearResponseExtraField(body, "max_output_tokens")
		return nil
	}
	floating, ok := number.Float64()
	if !ok {
		return errors.New("openai: invalid maxOutputTokens")
	}
	body.MaxOutputTokens = param.Opt[int64]{}
	setResponseExtraField(body, "max_output_tokens", floating)
	return nil
}
