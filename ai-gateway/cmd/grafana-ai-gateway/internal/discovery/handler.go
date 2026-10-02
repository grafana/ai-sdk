package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
)

const (
	// MaxModelRows bounds canonical and alias rows in one document.
	MaxModelRows = 1024
	// MaxCandidates bounds configured primary and fallback destinations per route.
	MaxCandidates = 16
	// MaxAliases bounds aliases configured for one route.
	MaxAliases = 128
	// MaxStringBytes bounds each recognized UTF-8 identity or display value.
	MaxStringBytes = 2048
)

var errResponseLimit = errors.New("gateway discovery: response exceeds byte limit")

type model struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Description   string           `json:"description,omitempty"`
	Specification specification    `json:"specification"`
	Gateway       *configuredRoute `json:"gateway,omitempty"`
}

type configuredRoute struct {
	CanonicalModelID string                `json:"canonicalModelId"`
	Aliases          []string              `json:"aliases"`
	Candidates       []configuredCandidate `json:"candidates"`
}

type configuredCandidate struct {
	ProviderInstance string `json:"providerInstance"`
	Provider         string `json:"provider"`
	ModelID          string `json:"modelId"`
}

type specification struct {
	SpecificationVersion string `json:"specificationVersion"`
	Provider             string `json:"provider"`
	ModelID              string `json:"modelId"`
}

type handler struct {
	lister catalog.ModelLister
	errors *providerv4.HostErrorWriter
	limit  int64
}

// New constructs a closed bounded discovery handler.
func New(lister catalog.ModelLister, errorWriter *providerv4.HostErrorWriter, limit int64) (http.Handler, error) {
	if limit <= 0 || limit == math.MaxInt64 {
		return nil, fmt.Errorf("gateway discovery: response limit is unsafe")
	}
	return &handler{lister: lister, errors: errorWriter, limit: limit}, nil
}

func (handler *handler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	models, err := handler.safeList(request.Context())
	if err != nil {
		handler.errors.Write(w, providerv4.HostErrorInternal)
		return
	}
	document, err := handler.encode(models)
	if err != nil {
		handler.errors.Write(w, providerv4.HostErrorInternal)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(document)
}

func (handler *handler) safeList(ctx context.Context) (models []catalog.ModelInfo, err error) {
	defer func() {
		if recover() != nil {
			models = nil
			err = fmt.Errorf("gateway discovery: listing models panicked")
		}
	}()
	return handler.lister.ListModels(ctx)
}

// Validate checks complete configured discovery feasibility before readiness.
func Validate(infos []catalog.ModelInfo, limit int64) error {
	if limit <= 0 || limit == math.MaxInt64 {
		return errResponseLimit
	}
	_, err := prepareRows(infos, limit)
	return err
}

func (handler *handler) encode(infos []catalog.ModelInfo) ([]byte, error) {
	rows, err := prepareRows(infos, handler.limit)
	if err != nil {
		return nil, err
	}
	document := []byte(`{"models":[`)
	for index, row := range rows {
		encoded, err := encodeModel(row)
		if err != nil {
			return nil, err
		}
		size := int64(len(encoded))
		if index > 0 {
			size++
		}
		if size > handler.limit-int64(len(document))-2 {
			return nil, errResponseLimit
		}
		if index > 0 {
			document = append(document, ',')
		}
		document = append(document, encoded...)
	}
	return append(document, ']', '}'), nil
}

func prepareRows(infos []catalog.ModelInfo, limit int64) ([]model, error) {
	rowCount, err := preflight(infos, limit)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, rowCount)
	models := make([]model, 0, len(infos))
	size := int64(len(`{"models":[]}`) + max(rowCount-1, 0))
	for _, info := range infos {
		if !validString(info.Name) || !utf8.ValidString(info.Description) {
			return nil, fmt.Errorf("gateway discovery: invalid model text")
		}
		if err := addPublicID(seen, info.ID); err != nil {
			return nil, err
		}
		for _, alias := range info.Aliases {
			if err := addPublicID(seen, alias); err != nil {
				return nil, err
			}
		}
		candidates := make(map[[2]string]struct{}, len(info.Candidates))
		for _, candidate := range info.Candidates {
			if !validString(candidate.ProviderInstance) || !validString(candidate.Provider) || !validString(candidate.ModelID) {
				return nil, fmt.Errorf("gateway discovery: invalid candidate")
			}
			key := [2]string{candidate.ProviderInstance, candidate.ModelID}
			if _, duplicate := candidates[key]; duplicate {
				return nil, fmt.Errorf("gateway discovery: duplicate candidate")
			}
			candidates[key] = struct{}{}
		}
		value := discoveryModel(info)
		encoded, err := encodeModel(value)
		if err != nil {
			return nil, err
		}
		groupSize := int64(len(encoded)) * int64(1+len(info.Aliases))
		for _, alias := range info.Aliases {
			groupSize += 2 * int64(len(alias)-len(info.ID))
		}
		if groupSize > limit-size {
			return nil, errResponseLimit
		}
		size += groupSize
		models = append(models, value)
	}
	rows := make([]model, 0, rowCount)
	for index, value := range models {
		rows = append(rows, value)
		for _, alias := range infos[index].Aliases {
			value.ID = alias
			value.Specification.ModelID = alias
			rows = append(rows, value)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows, nil
}

func preflight(infos []catalog.ModelInfo, limit int64) (int, error) {
	minimumSize := int64(len(`{"models":[]}`))
	if minimumSize > limit || len(infos) > MaxModelRows {
		return 0, errResponseLimit
	}
	rowCount := 0
	for _, info := range infos {
		if len(info.Aliases) > MaxAliases || len(info.Candidates) > MaxCandidates {
			return 0, errResponseLimit
		}
		count := 1 + len(info.Aliases)
		if count > MaxModelRows-rowCount {
			return 0, errResponseLimit
		}
		for _, value := range []string{info.ID, info.Name, info.Description} {
			if len(value) > MaxStringBytes {
				return 0, errResponseLimit
			}
		}
		commonSize := int64(len(info.Name) + len(info.Description))
		idSize := int64(2 * len(info.ID))
		if len(info.Candidates) != 0 {
			commonSize += int64(len(info.ID))
		}
		for _, alias := range info.Aliases {
			if len(alias) > MaxStringBytes {
				return 0, errResponseLimit
			}
			idSize += int64(2 * len(alias))
			if len(info.Candidates) != 0 {
				commonSize += int64(len(alias))
			}
		}
		for _, candidate := range info.Candidates {
			for _, value := range []string{candidate.ProviderInstance, candidate.Provider, candidate.ModelID} {
				if len(value) > MaxStringBytes {
					return 0, errResponseLimit
				}
				commonSize += int64(len(value))
			}
		}
		groupSize := commonSize*int64(count) + idSize
		if groupSize > limit-minimumSize {
			return 0, errResponseLimit
		}
		minimumSize += groupSize
		rowCount += count
	}
	return rowCount, nil
}

func addPublicID(seen map[string]struct{}, id string) error {
	if !validPublicID(id) {
		return fmt.Errorf("gateway discovery: invalid public ID")
	}
	if _, duplicate := seen[id]; duplicate {
		return fmt.Errorf("gateway discovery: duplicate public ID")
	}
	seen[id] = struct{}{}
	return nil
}

func discoveryModel(info catalog.ModelInfo) model {
	value := model{
		ID:          info.ID,
		Name:        info.Name,
		Description: info.Description,
		Specification: specification{
			SpecificationVersion: "v4",
			Provider:             "grafana",
			ModelID:              info.ID,
		},
	}
	if len(info.Candidates) != 0 {
		aliases := info.Aliases
		if aliases == nil {
			aliases = []string{}
		}
		candidates := make([]configuredCandidate, len(info.Candidates))
		for index, candidate := range info.Candidates {
			candidates[index] = configuredCandidate{ProviderInstance: candidate.ProviderInstance, Provider: candidate.Provider, ModelID: candidate.ModelID}
		}
		value.Gateway = &configuredRoute{CanonicalModelID: info.ID, Aliases: aliases, Candidates: candidates}
	}
	return value
}

func encodeModel(value model) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, fmt.Errorf("gateway discovery: encoding model: %w", err)
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

func validString(value string) bool {
	return strings.TrimSpace(value) != "" && utf8.ValidString(value)
}

func validPublicID(value string) bool {
	if len(value) < 1 || len(value) > 128 || !isASCIIAlphanumeric(value[0]) {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if !isASCIIAlphanumeric(character) && character != '.' && character != '_' && character != ':' && character != '/' && character != '-' {
			return false
		}
	}
	return true
}

func isASCIIAlphanumeric(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') || (value >= '0' && value <= '9')
}
