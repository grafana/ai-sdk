package enrichment

import (
	"fmt"
	"net/http"

	"github.com/grafana/ai-sdk/provider"
)

var builtInProtectedHeaders = []string{
	"Accept",
	"Authorization",
	"Proxy-Authorization",
	"Content-Type",
	"X-Access-Token",
	"X-Grafana-Id",
	"X-Api-Key",
	"Api-Key",
	"Openai-Api-Key",
	"Anthropic-Api-Key",
	"Ai-Language-Model-Id",
	"Ai-Language-Model-Streaming",
	"Ai-Language-Model-Specification-Version",
}

func headerOutputEnabled(opts HeaderOptions) bool {
	return len(opts.Map) > 0 || opts.Prefix != ""
}

func headerSelection(opts HeaderOptions) map[string]struct{} {
	if len(opts.Map) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(opts.Map))
	for key := range opts.Map {
		if key != "" {
			set[key] = struct{}{}
		}
	}
	return set
}

func applyHeaders(params provider.CallOptions, values []Value, include map[string]struct{}, opts HeaderOptions) (provider.CallOptions, error) {
	if !headerOutputEnabled(opts) || len(values) == 0 {
		return params, nil
	}

	headers := cloneHeaders(params.Headers)
	if headers == nil {
		headers = make(map[string]string)
	}
	existing := canonicalHeaderIndex(headers)
	protected := protectedHeaderSet(opts.AdditionalProtected)
	policy := canonicalConflictPolicy(opts.Conflict)

	changed := false
	for _, value := range values {
		target, ok := headerNameForValue(value, include, opts)
		if !ok || target == "" {
			continue
		}
		canonical := http.CanonicalHeaderKey(target)
		if _, ok := protected[canonical]; ok {
			continue
		}

		if existingName, ok := existing[canonical]; ok {
			switch policy {
			case ConflictEnrichmentWins:
				if existingName != canonical {
					delete(headers, existingName)
				}
				headers[canonical] = value.Value
				existing[canonical] = canonical
				changed = true
			case ConflictError:
				return params, fmt.Errorf("enrichment: header %q already exists", canonical)
			default:
				continue
			}
			continue
		}

		headers[canonical] = value.Value
		existing[canonical] = canonical
		changed = true
	}

	if !changed {
		return params, nil
	}
	params.Headers = headers
	return params, nil
}

func headerNameForValue(value Value, include map[string]struct{}, opts HeaderOptions) (string, bool) {
	if header, ok := opts.Map[value.Key]; ok {
		return header, true
	}
	if opts.Prefix == "" {
		return "", false
	}
	if _, ok := include[value.Key]; !ok {
		return "", false
	}
	return opts.Prefix + value.Key, true
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		out[key] = value
	}
	return out
}

func canonicalHeaderIndex(headers map[string]string) map[string]string {
	out := make(map[string]string, len(headers))
	for key := range headers {
		out[http.CanonicalHeaderKey(key)] = key
	}
	return out
}

func protectedHeaderSet(additional []string) map[string]struct{} {
	out := make(map[string]struct{}, len(builtInProtectedHeaders)+len(additional))
	for _, header := range builtInProtectedHeaders {
		out[http.CanonicalHeaderKey(header)] = struct{}{}
	}
	for _, header := range additional {
		if header != "" {
			out[http.CanonicalHeaderKey(header)] = struct{}{}
		}
	}
	return out
}
