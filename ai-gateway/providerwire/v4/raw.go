package v4

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
	"unicode/utf8"
)

// rawCredentialQueryKeys are the URL query parameters the Gateway developer
// evidence decision lists as authentication keys. Matching ignores case.
var rawCredentialQueryKeys = map[string]struct{}{
	"access_token":         {},
	"api_key":              {},
	"x-amz-credential":     {},
	"x-amz-signature":      {},
	"x-amz-security-token": {},
}

// projectRawValue returns the value to write for a requested raw stream part,
// or false when the part carries nothing safe to forward. A part whose native
// frame was not valid JSON reaches the Gateway with no value; a
// LanguageModelV4 raw part requires one and the Go client rejects a raw event
// without it, so such parts are dropped. Values that are not UTF-8 or that repeat an object key are dropped
// too: the first cannot be framed, and readers disagree on which duplicate
// wins, so a projected copy could still show another reader a credential.
//
// Raw values are native provider events, so the evidence decision's rule for
// native bodies applies: remove the credential-bearing fields that the
// producer's source shape identifies and keep everything else. The known echo
// is an OpenAI Responses lifecycle event, whose response object repeats the
// request's tools. Every object typed "mcp" is treated as such a definition,
// wherever it appears, and loses authorization and header values, and
// userinfo or authentication query keys in server_url.
func projectRawValue(raw json.RawMessage) (json.RawMessage, bool) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || !utf8.Valid(value) || !json.Valid(value) || hasDuplicateKeys(value) {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	var event any
	if decoder.Decode(&event) != nil {
		return nil, false
	}
	if !projectMCPDefinitions(event) {
		return value, true
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return nil, false
	}
	return encoded, true
}

// projectMCPDefinitions removes credentials from every MCP tool definition in
// a decoded JSON value and reports whether anything changed.
func projectMCPDefinitions(value any) bool {
	changed := false
	switch node := value.(type) {
	case []any:
		for _, item := range node {
			changed = projectMCPDefinitions(item) || changed
		}
	case map[string]any:
		for _, item := range node {
			changed = projectMCPDefinitions(item) || changed
		}
		if node["type"] != "mcp" {
			return changed
		}
		if _, present := node["authorization"]; present {
			delete(node, "authorization")
			changed = true
		}
		if headers, present := node["headers"]; present && headers != nil {
			node["headers"] = nil
			changed = true
		}
		if serverURL, present := node["server_url"]; present && serverURL != nil {
			text, ok := serverURL.(string)
			projected, keep, modified := projectCredentialURL(text)
			switch {
			case !ok || !keep:
				delete(node, "server_url")
				changed = true
			case modified:
				node["server_url"] = projected
				changed = true
			}
		}
	}
	return changed
}

// hasDuplicateKeys reports whether any object in a valid JSON value repeats a
// key after unescaping.
func hasDuplicateKeys(value []byte) bool {
	type container struct {
		keys      map[string]struct{}
		expectKey bool
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	var stack []*container
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return false
		}
		if err != nil {
			return true
		}
		var top *container
		if len(stack) > 0 {
			top = stack[len(stack)-1]
		}
		if top != nil && top.keys != nil && top.expectKey {
			key, ok := token.(string)
			if !ok {
				stack = stack[:len(stack)-1]
				continue
			}
			if _, duplicate := top.keys[key]; duplicate {
				return true
			}
			top.keys[key] = struct{}{}
			top.expectKey = false
			continue
		}
		if top != nil && top.keys != nil {
			top.expectKey = true
		}
		switch token {
		case json.Delim('{'):
			stack = append(stack, &container{keys: map[string]struct{}{}, expectKey: true})
		case json.Delim('['):
			stack = append(stack, &container{})
		case json.Delim(']'):
			stack = stack[:len(stack)-1]
		}
	}
}

// projectCredentialURL removes userinfo and authentication query keys from an
// echoed URL. A URL or query that cannot be parsed cannot be shown to be
// credential free, so it is not kept.
func projectCredentialURL(raw string) (projected string, keep, modified bool) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false, true
	}
	if parsed.User != nil {
		parsed.User = nil
		modified = true
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", false, true
	}
	for key := range query {
		if _, credential := rawCredentialQueryKeys[strings.ToLower(key)]; credential {
			query.Del(key)
			modified = true
		}
	}
	if !modified {
		return raw, true, false
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), true, true
}
