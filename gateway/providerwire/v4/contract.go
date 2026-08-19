package providerwirev4

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	validateschema "github.com/santhosh-tekuri/jsonschema/v6"
)

const embeddedSchemaIDPrefix = "https://grafana.com/ai-sdk/providerwire/v4/schema/"

//go:embed schema/*.json
var schemaFiles embed.FS

type schemaRegistry struct {
	compiled map[string]*validateschema.Schema
}

var (
	sharedRegistryOnce sync.Once
	sharedRegistry     *schemaRegistry
	sharedRegistryErr  error
)

func loadEmbeddedContractRegistry() (*schemaRegistry, error) {
	sharedRegistryOnce.Do(func() {
		sharedRegistry, sharedRegistryErr = compileEmbeddedContractRegistry()
	})
	return sharedRegistry, sharedRegistryErr
}

func compileEmbeddedContractRegistry() (*schemaRegistry, error) {
	entries, err := schemaFiles.ReadDir("schema")
	if err != nil {
		return nil, fmt.Errorf("providerwirev4: reading embedded schemas: %w", err)
	}
	type resource struct {
		id  string
		doc any
	}
	resources := make([]resource, 0, len(entries))
	known := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		raw, err := schemaFiles.ReadFile("schema/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("providerwirev4: reading embedded schema %q: %w", entry.Name(), err)
		}
		if _, err := validateStrictJSON(raw); err != nil {
			return nil, fmt.Errorf("providerwirev4: validating embedded schema %q: %w", entry.Name(), err)
		}
		var metadata struct {
			ID string `json:"$id"`
		}
		if err := json.Unmarshal(raw, &metadata); err != nil {
			return nil, fmt.Errorf("providerwirev4: decoding embedded schema metadata %q: %w", entry.Name(), err)
		}
		if !strings.HasPrefix(metadata.ID, embeddedSchemaIDPrefix) {
			return nil, fmt.Errorf("providerwirev4: unexpected schema id %q", metadata.ID)
		}
		if _, duplicate := known[metadata.ID]; duplicate {
			return nil, fmt.Errorf("providerwirev4: duplicate schema id %q", metadata.ID)
		}
		known[metadata.ID] = struct{}{}
		doc, err := validateschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("providerwirev4: decoding embedded schema %q: %w", entry.Name(), err)
		}
		resources = append(resources, resource{id: metadata.ID, doc: doc})
	}
	if len(resources) != 5 {
		return nil, fmt.Errorf("providerwirev4: expected 5 embedded schemas, got %d", len(resources))
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].id < resources[j].id })
	compiler := validateschema.NewCompiler()
	compiler.DefaultDraft(validateschema.Draft2020)
	compiler.AssertFormat()
	compiler.UseLoader(validateschema.SchemeURLLoader{})
	for _, resource := range resources {
		if err := compiler.AddResource(resource.id, resource.doc); err != nil {
			return nil, fmt.Errorf("providerwirev4: registering schema %q: %w", resource.id, err)
		}
	}
	compiled := make(map[string]*validateschema.Schema, len(resources)-1)
	for _, resource := range resources {
		schema, err := compiler.Compile(resource.id)
		if err != nil {
			return nil, fmt.Errorf("providerwirev4: compiling schema %q: %w", resource.id, err)
		}
		name := strings.TrimSuffix(strings.TrimPrefix(resource.id, embeddedSchemaIDPrefix), ".json")
		if name != "common" {
			compiled[name] = schema
		}
	}
	return &schemaRegistry{compiled: compiled}, nil
}

func (r *schemaRegistry) validate(name string, raw json.RawMessage) error {
	if _, err := validateStrictJSON(raw); err != nil {
		return fmt.Errorf("syntax: %w", err)
	}
	return r.validateSyntaxChecked(name, raw)
}

func (r *schemaRegistry) validateSyntaxChecked(name string, raw json.RawMessage) error {
	schema, ok := r.compiled[name]
	if !ok {
		return fmt.Errorf("unknown contract schema %q", name)
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("semantic json: %w", err)
	}
	return schema.Validate(value)
}

func (r *schemaRegistry) validateErrorEnvelope(raw json.RawMessage, status int) error {
	if err := r.validate("error", raw); err != nil {
		return err
	}
	var payload struct {
		Error struct {
			StatusCode int `json:"statusCode"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("decoding error envelope: %w", err)
	}
	if payload.Error.StatusCode != status {
		return fmt.Errorf("status correlation: HTTP status %d differs from nested status %d", status, payload.Error.StatusCode)
	}
	return nil
}

func safeValidationPath(err error) string {
	var validationErr *validateschema.ValidationError
	if !errors.As(err, &validationErr) || validationErr == nil {
		return ""
	}
	return safeJSONPointer(validationErr.InstanceLocation)
}

func safeJSONPointer(segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	encoded := make([]string, len(segments))
	for i, segment := range segments {
		encoded[i] = strings.ReplaceAll(strings.ReplaceAll(segment, "~", "~0"), "/", "~1")
	}
	return "/" + strings.Join(encoded, "/")
}
