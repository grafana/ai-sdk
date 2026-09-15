package grafana

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

// ModelSpecification identifies the public LanguageModelV4 model.
type ModelSpecification struct {
	SpecificationVersion string `json:"specificationVersion"`
	Provider             string `json:"provider"`
	ModelID              string `json:"modelId"`
}

// ModelInfo is a public catalog row. Unknown server metadata is not exposed.
type ModelInfo struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Description   *string            `json:"description,omitempty"`
	Specification ModelSpecification `json:"specification"`
}

// ListModels returns the authenticated catalog in server order, without caching.
// A malformed row or response invalidates the entire result.
func (p *Provider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := p.request(ctx, http.MethodGet, "/config")
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		retryable := true
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: "grafana: discovery transport failed", Cause: err, IsRetryable: &retryable})
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, readGatewayError(ctx, resp, p.limits.ErrorBytes)
	}
	body, err := readJSON(ctx, resp, p.limits.DiscoveryBytes)
	if err != nil {
		return nil, protocolError("grafana: invalid discovery response", resp.StatusCode, err)
	}
	var document struct {
		Models *[]json.RawMessage `json:"models"`
	}
	if err := decodeFields(body, &document, "models"); err != nil || document.Models == nil {
		return nil, protocolError("grafana: invalid discovery document", resp.StatusCode, nil)
	}
	rows := make([]ModelInfo, 0, len(*document.Models))
	seen := make(map[string]struct{}, len(*document.Models))
	for _, raw := range *document.Models {
		var row ModelInfo
		var fields struct {
			ID            string          `json:"id"`
			Name          string          `json:"name"`
			Description   json.RawMessage `json:"description"`
			Specification json.RawMessage `json:"specification"`
		}
		if decodeFields(raw, &fields, "id", "name", "description", "specification") != nil || decodeFields(fields.Specification, &row.Specification, "specificationVersion", "provider", "modelId") != nil {
			return nil, protocolError("grafana: invalid discovery model", resp.StatusCode, nil)
		}
		row.ID, row.Name = fields.ID, fields.Name
		if len(fields.Description) > 0 {
			var description *string
			if json.Unmarshal(fields.Description, &description) != nil {
				return nil, protocolError("grafana: invalid discovery description", resp.StatusCode, nil)
			}
			row.Description = description
		}
		_, duplicate := seen[row.ID]
		if !publicModelID.MatchString(row.ID) || !validPublicText(row.Name) || row.Description != nil && !utf8.ValidString(*row.Description) || row.Specification.SpecificationVersion != "v4" || row.Specification.Provider != "grafana" || row.Specification.ModelID != row.ID || duplicate {
			return nil, protocolError("grafana: invalid discovery model", resp.StatusCode, nil)
		}
		seen[row.ID] = struct{}{}
		rows = append(rows, row)
	}
	return rows, nil
}

func validPublicText(value string) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != ""
}

func readJSON(ctx context.Context, resp *http.Response, limit int64) ([]byte, error) {
	media, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || media != "application/json" && (!strings.HasPrefix(media, "application/") || !strings.HasSuffix(media, "+json")) {
		return nil, errors.New("grafana: expected JSON media type")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("grafana: response byte limit exceeded")
	}
	if !validJSON(body) {
		return nil, errors.New("grafana: malformed JSON response")
	}
	return body, nil
}

func protocolError(message string, status int, cause error) *provider.APICallError {
	retryable := false
	return provider.NewAPICallError(provider.APICallErrorOptions{Message: message, StatusCode: status, Cause: cause, IsRetryable: &retryable})
}
