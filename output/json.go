package output

import (
	"encoding/json"
	"fmt"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

// JSONOutput implements aisdk.Output for generating unstructured but valid JSON.
// No schema constraint is applied; the response is only validated to be valid JSON.
type JSONOutput struct {
	name *string
	desc *string
}

// JSON creates a JSONOutput that requests JSON mode from the provider
// without a schema constraint.
func JSON(opts ...ObjectOption) *JSONOutput {
	o := &JSONOutput{}
	for _, opt := range opts {
		opt.applyObject(o)
	}
	return o
}

func (o *JSONOutput) setName(name string)        { o.name = &name }
func (o *JSONOutput) setDescription(desc string) { o.desc = &desc }
func (o *JSONOutput) ResponseFormat() *provider.ResponseFormat {
	return &provider.ResponseFormat{
		Type:        provider.ResponseFormatJSON,
		Name:        o.name,
		Description: o.desc,
	}
}

func (o *JSONOutput) ParseComplete(text string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON: %v", aisdk.ErrNoObjectGenerated, err)
	}
	return v, nil
}

func (o *JSONOutput) ParsePartial(text string) (any, bool) {
	return parsePartialJSON(text)
}
