package evidence

import (
	"bytes"
	"encoding/json/jsontext"
)

type errorProjection struct {
	value    []byte
	redacted bool
	echo     bool
}

func protectedErrorJSON(source []byte, sources []string) (errorProjection, error) {
	decoder := jsontext.NewDecoder(bytes.NewReader(source), jsontext.AllowInvalidUTF8(true))
	var output bytes.Buffer
	encoder := jsontext.NewEncoder(&output, jsontext.AllowInvalidUTF8(true), jsontext.PreserveRawStrings(true), jsontext.EscapeForHTML(true))
	var projection errorProjection
	if err := projectErrorJSON(decoder, encoder, sources, &projection); err != nil {
		return projection, err
	}
	projection.value = bytes.TrimSpace(output.Bytes())
	return projection, nil
}

func projectErrorJSON(decoder *jsontext.Decoder, encoder *jsontext.Encoder, sources []string, projection *errorProjection) error {
	token, err := decoder.ReadToken()
	if err != nil {
		return err
	}
	kind := token.Kind()
	if kind == '"' && containsProtectedText(token.String(), sources) {
		projection.echo = true
	}
	if err := encoder.WriteToken(token); err != nil {
		return err
	}
	if kind != '{' && kind != '[' {
		return nil
	}
	end := jsontext.Kind('}')
	if kind == '[' {
		end = ']'
	}
	for decoder.PeekKind() != end {
		if kind == '{' {
			name, err := decoder.ReadToken()
			if err != nil {
				return err
			}
			if protectedErrorField(name.String()) {
				projection.redacted = true
				if err := decoder.SkipValue(); err != nil {
					return err
				}
				continue
			}
			if containsProtectedText(name.String(), sources) {
				projection.echo = true
			}
			if err := encoder.WriteToken(name); err != nil {
				return err
			}
		}
		if err := projectErrorJSON(decoder, encoder, sources, projection); err != nil {
			return err
		}
	}
	token, err = decoder.ReadToken()
	if err != nil {
		return err
	}
	return encoder.WriteToken(token)
}
