package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

type guardPhase string

const (
	guardPreflight  guardPhase = "preflight"
	guardPostflight guardPhase = "postflight"
)

var (
	errGuardResponse    = errors.New("service: invalid guard response")
	errGuardTransform   = errors.New("service: unusable guard transform")
	errGuardUnsupported = errors.New("service: unsupported guard input")
)

type guardAction string

const (
	guardAllow guardAction = "allow"
	guardDeny  guardAction = "deny"
)

type guardVerdict struct {
	action           guardAction
	transform        json.RawMessage
	transformPresent bool
}

func decodeGuardJSONContext(ctx context.Context, raw []byte, out any) error {
	value, err := guardJSONValueContext(ctx, raw)
	if err != nil {
		return err
	}
	if !guardJSONShape(ctx, value, reflect.TypeOf(out).Elem()) {
		if err := ctx.Err(); err != nil {
			return err
		}
		return errGuardTransform
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := json.NewDecoder(&guardContextReader{ctx: ctx, reader: bytes.NewReader(raw)}).Decode(out); err != nil {
		return err
	}
	return ctx.Err()
}

func guardJSONShape(ctx context.Context, value any, t reflect.Type) bool {
	if ctx.Err() != nil {
		return false
	}
	if t == reflect.TypeOf(json.RawMessage{}) {
		return true
	}
	if t.Kind() == reflect.Pointer {
		return value != nil && guardJSONShape(ctx, value, t.Elem())
	}
	if value == nil {
		return false
	}
	switch t.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			return false
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name != "" && name != "-" {
				fields[name] = f.Type
			}
		}
		for name, v := range object {
			field, ok := fields[name]
			if !ok || !guardJSONShape(ctx, v, field) {
				return false
			}
		}
		return true
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			s, ok := value.(string)
			if !ok {
				return false
			}
			decoded, err := base64.StdEncoding.Strict().DecodeString(s)
			return ctx.Err() == nil && err == nil && base64.StdEncoding.EncodeToString(decoded) == s
		}
		array, ok := value.([]any)
		if !ok {
			return false
		}
		for _, v := range array {
			if !guardJSONShape(ctx, v, t.Elem()) {
				return false
			}
		}
		return true
	case reflect.String:
		_, ok := value.(string)
		return ok
	case reflect.Bool:
		_, ok := value.(bool)
		return ok
	case reflect.Int, reflect.Int64:
		_, ok := value.(json.Number)
		return ok
	}
	return false
}

func guardJSONValueContext(ctx context.Context, raw []byte) (any, error) {
	if !guardValidStrings(ctx, raw) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, errGuardTransform
	}
	dec := json.NewDecoder(&guardContextReader{ctx: ctx, reader: bytes.NewReader(raw)})
	dec.UseNumber()
	value, err := guardJSONToken(ctx, dec, 0)
	if err != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, errGuardTransform
	}
	if _, err = dec.Token(); err != io.EOF {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, errGuardTransform
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return value, nil
}

func guardJSONToken(ctx context.Context, dec *json.Decoder, depth int) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if depth > 128 {
		return nil, errGuardTransform
	}
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch v := token.(type) {
	case json.Delim:
		switch v {
		case '{':
			object := map[string]any{}
			for dec.More() {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				keyToken, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, errGuardTransform
				}
				if _, ok := object[key]; ok {
					return nil, errGuardTransform
				}
				value, err := guardJSONToken(ctx, dec, depth+1)
				if err != nil {
					return nil, err
				}
				object[key] = value
			}
			end, err := dec.Token()
			if err != nil || end != json.Delim('}') {
				return nil, errGuardTransform
			}
			return object, nil
		case '[':
			array := []any{}
			for dec.More() {
				value, err := guardJSONToken(ctx, dec, depth+1)
				if err != nil {
					return nil, err
				}
				array = append(array, value)
			}
			end, err := dec.Token()
			if err != nil || end != json.Delim(']') {
				return nil, errGuardTransform
			}
			return array, nil
		}
		return nil, errGuardTransform
	case json.Number:
		if !guardNumberBounds(v) {
			return nil, errGuardTransform
		}
	}
	return token, nil
}

func guardNumberBounds(n json.Number) bool {
	s := string(n)
	if len(s) > 4096 {
		return false
	}
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		e, err := strconv.Atoi(s[i+1:])
		if err != nil || e > 10000 || e < -10000 {
			return false
		}
	}
	return true
}

type guardDecimal struct {
	negative bool
	digits   string
	exponent int
}

func guardNumber(n json.Number) guardDecimal {
	s := string(n)
	value := guardDecimal{negative: strings.HasPrefix(s, "-")}
	s = strings.TrimPrefix(s, "-")
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		value.exponent, _ = strconv.Atoi(s[i+1:])
		s = s[:i]
	}
	if i := strings.IndexByte(s, '.'); i >= 0 {
		value.exponent -= len(s) - i - 1
		s = s[:i] + s[i+1:]
	}
	s = strings.TrimLeft(s, "0")
	if s == "" {
		return guardDecimal{}
	}
	value.digits = strings.TrimRight(s, "0")
	value.exponent += len(s) - len(value.digits)
	return value
}

func guardValidStrings(ctx context.Context, raw []byte) bool {
	for i := 0; i < len(raw); {
		if ctx.Err() != nil {
			return false
		}
		end := min(i+4096, len(raw))
		if end < len(raw) {
			for end > i && !utf8.RuneStart(raw[end]) {
				end--
			}
		}
		if end == i || !utf8.Valid(raw[i:end]) {
			return false
		}
		i = end
	}
	for i := 0; i < len(raw); i++ {
		if i%4096 == 0 && ctx.Err() != nil {
			return false
		}
		if raw[i] != '"' {
			continue
		}
		i++
		for ; i < len(raw) && raw[i] != '"'; i++ {
			if i%4096 == 0 && ctx.Err() != nil {
				return false
			}
			if raw[i] != '\\' {
				continue
			}
			i++
			if i >= len(raw) {
				return false
			}
			if raw[i] != 'u' {
				continue
			}
			if i+4 >= len(raw) {
				return false
			}
			n, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
			if err != nil {
				return false
			}
			i += 4
			if n >= 0xdc00 && n <= 0xdfff {
				return false
			}
			if n >= 0xd800 && n <= 0xdbff {
				if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
					return false
				}
				low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
				if err != nil || low < 0xdc00 || low > 0xdfff {
					return false
				}
				i += 6
			}
		}
	}
	return ctx.Err() == nil
}

type guardContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *guardContextReader) Read(p []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(p[:min(len(p), 4096)])
}
