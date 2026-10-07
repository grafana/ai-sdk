package logger

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
)

const (
	maxCaptureDepth = 64
	maxCaptureNodes = 65536
	maxCaptureBytes = 4 << 20
)

var errCaptureBounds = errors.New("logger: capture cannot be safely normalized")

type captureNormalizer struct {
	remaining int
	bytes     int
}

func normalizeCapture(value any) (result any, err error) {
	defer func() {
		if recover() != nil {
			result, err = nil, errCaptureBounds
		}
	}()
	n := captureNormalizer{remaining: maxCaptureNodes, bytes: maxCaptureBytes}
	return n.normalize(reflect.ValueOf(value), 0, false)
}

func (n *captureNormalizer) normalize(value reflect.Value, depth int, gateway bool) (any, error) {
	n.remaining--
	if depth > maxCaptureDepth || n.remaining < 0 {
		return nil, errCaptureBounds
	}
	if !value.IsValid() {
		return nil, nil
	}
	if value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, nil
		}
		return n.normalize(value.Elem(), depth+1, gateway)
	}
	if (value.Kind() == reflect.Slice || value.Kind() == reflect.Map) && value.IsNil() {
		return nil, nil
	}
	if value.Kind() == reflect.Slice && value.Type().Elem().Kind() == reflect.Uint8 {
		return n.decode(value.Bytes(), depth, gateway)
	}
	if marshaler, ok := value.Interface().(json.Marshaler); ok {
		data, err := marshaler.MarshalJSON()
		if err != nil {
			return nil, err
		}
		return n.decode(data, depth, gateway)
	}
	if gateway && value.Kind() != reflect.Map && value.Kind() != reflect.Struct {
		return redactedValue, nil
	}
	switch value.Kind() {
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String || value.Len() > n.remaining {
			return nil, errCaptureBounds
		}
		out := make(map[string]any, value.Len())
		iter := value.MapRange()
		for iter.Next() {
			key := iter.Key().String()
			n.bytes -= len(key)
			if n.bytes < 0 {
				return nil, errCaptureBounds
			}
			if gateway && strings.EqualFold(key, "byok") {
				out[key] = redactedValue
				continue
			}
			item, err := n.normalize(iter.Value(), depth+1, strings.EqualFold(key, "gateway"))
			if err != nil {
				return nil, err
			}
			out[key] = item
		}
		return out, nil
	case reflect.Array, reflect.Slice:
		if value.Len() > n.remaining {
			return nil, errCaptureBounds
		}
		out := make([]any, value.Len())
		for i := range out {
			item, err := n.normalize(value.Index(i), depth+1, false)
			if err != nil {
				return nil, err
			}
			out[i] = item
		}
		return out, nil
	case reflect.Struct:
		data, err := json.Marshal(value.Interface())
		if err != nil {
			return nil, err
		}
		return n.decode(data, depth, gateway)
	case reflect.String:
		n.bytes -= value.Len()
		if n.bytes < 0 {
			return nil, errCaptureBounds
		}
	}
	return value.Interface(), nil
}

func (n *captureNormalizer) decode(data []byte, depth int, gateway bool) (any, error) {
	n.bytes -= len(data)
	if n.bytes < 0 {
		return nil, errCaptureBounds
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, err
	}
	return n.normalize(reflect.ValueOf(decoded), depth+1, gateway)
}
