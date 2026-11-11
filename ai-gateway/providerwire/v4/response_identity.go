package v4

import (
	"time"
	"unicode/utf8"
)

type responseIdentity struct {
	ID        string `json:"id,omitempty"`
	ModelID   string `json:"modelId,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

func validResponseTimestamp(value time.Time) bool {
	if value.IsZero() {
		return true
	}
	year := value.UTC().Year()
	return year >= 0 && year <= 9999
}

func responseTimestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func mapResponseIdentity(id, modelID string, timestamp time.Time) (responseIdentity, error) {
	if !utf8.ValidString(id) || !utf8.ValidString(modelID) || !validResponseTimestamp(timestamp) {
		return responseIdentity{}, errInvalidUnarySuccess
	}
	return responseIdentity{ID: id, ModelID: modelID, Timestamp: responseTimestamp(timestamp)}, nil
}
