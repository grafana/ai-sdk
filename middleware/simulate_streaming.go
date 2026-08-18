package middleware

import (
	"context"
	"strconv"

	"github.com/grafana/ai-sdk/provider"
)

// SimulateStreaming returns a Middleware that intercepts DoStream calls,
// calls DoGenerate on the inner model instead, and converts the generate
// result into a synthetic stream of provider.StreamPart events.
//
// DoGenerate calls pass through unmodified.
func SimulateStreaming() Middleware {
	return Middleware{
		WrapStream: func(ctx context.Context, params WrapStreamParams) (*provider.StreamResult, error) {
			result, err := params.DoGenerate(ctx)
			if err != nil {
				return nil, err
			}

			ch := make(chan provider.StreamPart, 64)
			go func() {
				defer close(ch)

				ch <- provider.StreamPart{
					Type:     provider.PartStreamStart,
					Warnings: result.Warnings,
				}

				responseMeta := provider.StreamPart{Type: provider.PartResponseMeta}
				if result.Response != nil {
					responseMeta.ResponseID = result.Response.ID
					responseMeta.ModelID = result.Response.ModelID
					responseMeta.Timestamp = result.Response.Timestamp
					responseMeta.ResponseHeaders = result.Response.Headers
				}
				ch <- responseMeta

				id := 0
				for _, part := range result.Content {
					idStr := strconv.Itoa(id)
					switch part.Type {
					case provider.ContentText:
						if len(part.Text) > 0 {
							ch <- provider.StreamPart{Type: provider.PartTextStart, ID: idStr}
							ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: idStr, Delta: part.Text}
							ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: idStr}
							id++
						}
					case provider.ContentReasoning:
						ch <- provider.StreamPart{Type: provider.PartReasoningStart, ID: idStr, ProviderMetadata: part.ProviderMetadata}
						ch <- provider.StreamPart{Type: provider.PartReasoningDelta, ID: idStr, Delta: part.Text}
						ch <- provider.StreamPart{Type: provider.PartReasoningEnd, ID: idStr}
						id++
					default:
						ch <- contentPartToStreamPart(part)
					}
				}

				ch <- provider.StreamPart{
					Type:             provider.PartFinish,
					FinishReason:     &result.FinishReason,
					Usage:            &result.Usage,
					ProviderMetadata: result.ProviderMetadata,
				}
			}()

			var req *provider.RequestMetadata
			var resp *provider.ResponseHeaders
			if result.Request != nil {
				req = result.Request
			}
			if result.Response != nil {
				resp = &provider.ResponseHeaders{
					Headers: result.Response.Headers,
				}
			}

			return &provider.StreamResult{
				Stream:   ch,
				Request:  req,
				Response: resp,
			}, nil
		},
	}
}

func contentPartToStreamPart(part provider.GenerateContentPart) provider.StreamPart {
	sp := provider.StreamPart{
		Type:             provider.StreamPartType(string(part.Type)),
		Kind:             part.Kind,
		ToolCallID:       part.ToolCallID,
		ToolName:         part.ToolName,
		ProviderExecuted: part.ProviderExecuted,
		Dynamic:          part.Dynamic,
		MediaType:        part.MediaType,
		Filename:         part.Filename,
		ProviderMetadata: part.ProviderMetadata,
	}

	if len(part.Input) > 0 {
		sp.Input = string(part.Input)
	}

	if part.SourceType != "" || part.URL != "" {
		sp.Source = &provider.SourceInfo{
			SourceType: part.SourceType,
			URL:        part.URL,
		}
	}

	if part.Data != nil {
		switch {
		case part.Data.Bytes != nil:
			sp.Data = &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Bytes: part.Data.Bytes}
		case part.Data.Base64 != "":
			sp.Data = &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Base64: part.Data.Base64}
		case part.Data.URL != "":
			sp.Data = &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: part.Data.URL}
		}
	}

	return sp
}
