//go:build conformance

package conformance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stretchr/testify/assert"
)

type comparisonLog struct {
	errors []string
}

func (l *comparisonLog) Helper() {}
func (l *comparisonLog) Errorf(format string, args ...any) {
	l.errors = append(l.errors, fmt.Sprintf(format, args...))
}

func CompareScenario(dir string, result ScenarioResult, requests []RequestSnapshot) ([]string, error) {
	cfg, err := LoadConfig(filepath.Join(dir, "config.yaml"))
	if err != nil {
		return nil, err
	}
	log := &comparisonLog{errors: []string{}}
	if cfg.ExpectStreamError {
		assert.NotEmpty(log, result.Error, "expected stream error")
	} else {
		assert.Empty(log, result.Error, "unexpected execution error")
	}
	if cfg.Operation == OperationGenerate {
		if err := compareJSONArtifact(log, filepath.Join(dir, "expected-generate.json"), result.Generate); err != nil {
			return nil, err
		}
	} else {
		expected, err := LoadExpected(filepath.Join(dir, "expected.jsonl"))
		if err != nil {
			return nil, err
		}
		CompareChunks(log, expected, result.Chunks)
	}
	expectedRequests, err := LoadExpectedRequests(filepath.Join(dir, "expected-requests.jsonl"))
	if err != nil {
		return nil, err
	}
	for i := range requests {
		body, err := json.Marshal(requests[i].Body)
		if err != nil {
			return nil, err
		}
		requests[i].Body, err = decodeJSONBody(body)
		if err != nil {
			return nil, err
		}
	}
	CompareRequestSnapshots(log, expectedRequests, requests)
	for _, artifact := range []struct {
		name  string
		value any
	}{
		{"expected-usage.json", result.Usage},
		{"expected-object.json", result.Object},
	} {
		path := filepath.Join(dir, artifact.name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		value := artifact.value
		if artifact.name == "expected-object.json" && value == nil && !cfg.AssertOutputValue {
			var text strings.Builder
			for _, chunk := range result.Chunks {
				if chunk["type"] == "text-delta" {
					delta, _ := chunk["delta"].(string)
					text.WriteString(delta)
				}
			}
			if err := json.Unmarshal([]byte(text.String()), &value); err != nil {
				log.Errorf("reconstructing output object: %v", err)
			}
		}
		if err := compareJSONArtifact(log, path, value); err != nil {
			return nil, err
		}
	}
	return log.errors, nil
}

func compareJSONArtifact(log *comparisonLog, path string, value any) error {
	expected, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	actual, err := json.Marshal(value)
	if err != nil {
		return err
	}
	assert.JSONEq(log, string(expected), string(actual), filepath.Base(path))
	return nil
}
