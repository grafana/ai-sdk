// Package azure provides language models hosted on Microsoft Azure.
package azure

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/grafana/ai-sdk/providers/azure/foundry"
)

// NewAnthropic creates a Claude model hosted by Microsoft Foundry. modelID is
// the canonical Claude ID used for capabilities and response metadata;
// deploymentName is the name sent to Foundry. Both must be explicit and
// non-empty. Availability and deployment approval are caller policy.
func NewAnthropic(config foundry.Config, modelID, deploymentName string, opts ...anthropic.Option) (provider.LanguageModel, error) {
	if modelID == "" || strings.TrimSpace(modelID) != modelID || deploymentName == "" || strings.TrimSpace(deploymentName) != deploymentName {
		return nil, fmt.Errorf("azure: model ID and deployment name must be non-empty without surrounding whitespace")
	}
	requestOpts, err := config.RequestOptions()
	if err != nil {
		return nil, err
	}
	// The compatible published adapter loads native SDK defaults at construction.
	// Remove ambient custom headers before explicit caller options are applied.
	for line := range strings.SplitSeq(os.Getenv("ANTHROPIC_CUSTOM_HEADERS"), "\n") {
		if name, _, ok := strings.Cut(line, ":"); ok {
			requestOpts = append(requestOpts, option.WithHeaderDel(strings.TrimSpace(name)))
		}
	}
	modelOpts := []anthropic.Option{anthropic.WithRequestOptions(requestOpts...)}
	modelOpts = append(modelOpts, opts...)
	modelOpts = append(modelOpts, anthropic.WithRequestOptions(option.WithJSONSet("model", deploymentName)))
	return &anthropicModel{LanguageModel: anthropic.New("", modelID, modelOpts...)}, nil
}

type anthropicModel struct {
	provider.LanguageModel
}

func (m *anthropicModel) Provider() string { return "azure" }

func (m *anthropicModel) DoGenerate(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
	result, err := m.LanguageModel.DoGenerate(ctx, params)
	if err == nil && result != nil && result.Response != nil {
		result.Response.Provider = m.Provider()
		result.Response.ModelID = m.ModelID()
	}
	return result, err
}

func (m *anthropicModel) DoStream(ctx context.Context, params provider.CallOptions) (*provider.StreamResult, error) {
	result, err := m.LanguageModel.DoStream(ctx, params)
	if err != nil || result == nil || result.Stream == nil {
		return result, err
	}
	out := *result
	parts := make(chan provider.StreamPart, cap(result.Stream))
	out.Stream = parts
	go func() {
		defer close(parts)
		for part := range result.Stream {
			if part.Type == provider.PartResponseMeta {
				part.Provider = m.Provider()
				part.ModelID = m.ModelID()
			}
			select {
			case parts <- part:
			case <-ctx.Done():
				for range result.Stream {
				}
				return
			}
		}
	}()
	return &out, nil
}
