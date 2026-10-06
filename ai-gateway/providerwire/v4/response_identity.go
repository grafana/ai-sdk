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

func responseStringsFit(remaining *int64, values ...string) bool {
	for _, value := range values {
		if int64(len(value)) > *remaining {
			return false
		}
		*remaining -= int64(len(value))
	}
	return true
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

func responseIdentityFits(id, modelID string, timestamp time.Time, remaining *int64) bool {
	return validResponseTimestamp(timestamp) && responseStringsFit(remaining, id, modelID, responseTimestamp(timestamp))
}

func mapResponseIdentity(id, modelID string, timestamp time.Time) (responseIdentity, error) {
	if !utf8.ValidString(id) || !utf8.ValidString(modelID) || !validResponseTimestamp(timestamp) {
		return responseIdentity{}, errInvalidUnarySuccess
	}
	return responseIdentity{ID: id, ModelID: modelID, Timestamp: responseTimestamp(timestamp)}, nil
}
