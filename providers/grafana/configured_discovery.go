package grafana

import (
	"encoding/json"
	"errors"
	"slices"
)

const (
	maxDiscoveryRows        = 1024
	maxDiscoveryCandidates  = 16
	maxDiscoveryAliases     = 128
	maxDiscoveryStringBytes = 2048
)

var errConfiguredRoute = errors.New("grafana: invalid configured route")

func validDiscoveryString(value string) bool {
	return len(value) <= maxDiscoveryStringBytes && validPublicText(value)
}

func decodeConfiguredRoute(raw json.RawMessage) (*ConfiguredRoute, error) {
	var fields struct {
		CanonicalModelID string             `json:"canonicalModelId"`
		Aliases          *[]json.RawMessage `json:"aliases"`
		Candidates       *[]json.RawMessage `json:"candidates"`
	}
	if decodeDiscoveryFields(raw, &fields, "canonicalModelId", "aliases", "candidates") != nil || !publicModelID.MatchString(fields.CanonicalModelID) || fields.Aliases == nil || fields.Candidates == nil || len(*fields.Aliases) > maxDiscoveryAliases || len(*fields.Candidates) == 0 || len(*fields.Candidates) > maxDiscoveryCandidates {
		return nil, errConfiguredRoute
	}
	route := &ConfiguredRoute{CanonicalModelID: fields.CanonicalModelID, Aliases: make([]string, 0, len(*fields.Aliases)), Candidates: make([]ConfiguredCandidate, 0, len(*fields.Candidates))}
	aliases := make(map[string]struct{}, len(*fields.Aliases))
	for _, rawAlias := range *fields.Aliases {
		var alias string
		if !validDiscoveryStringJSON(rawAlias) || json.Unmarshal(rawAlias, &alias) != nil || !publicModelID.MatchString(alias) || alias == route.CanonicalModelID {
			return nil, errConfiguredRoute
		}
		if _, duplicate := aliases[alias]; duplicate {
			return nil, errConfiguredRoute
		}
		aliases[alias] = struct{}{}
		route.Aliases = append(route.Aliases, alias)
	}
	candidates := make(map[[2]string]struct{}, len(*fields.Candidates))
	for _, rawCandidate := range *fields.Candidates {
		var candidate ConfiguredCandidate
		if decodeDiscoveryFields(rawCandidate, &candidate, "providerInstance", "provider", "modelId") != nil || !validDiscoveryString(candidate.ProviderInstance) || !validDiscoveryString(candidate.Provider) || !validDiscoveryString(candidate.ModelID) {
			return nil, errConfiguredRoute
		}
		key := [2]string{candidate.ProviderInstance, candidate.ModelID}
		if _, duplicate := candidates[key]; duplicate {
			return nil, errConfiguredRoute
		}
		candidates[key] = struct{}{}
		route.Candidates = append(route.Candidates, candidate)
	}
	return route, nil
}

func sameConfiguredRoute(a, b *ConfiguredRoute) bool {
	return a != nil && b != nil && a.CanonicalModelID == b.CanonicalModelID && slices.Equal(a.Aliases, b.Aliases) && slices.Equal(a.Candidates, b.Candidates)
}

func validConfiguredGroups(rows []ModelInfo) bool {
	byID := make(map[string]*ConfiguredRoute, len(rows))
	for _, row := range rows {
		byID[row.ID] = row.Gateway
	}
	for _, row := range rows {
		route := row.Gateway
		if route == nil {
			continue
		}
		if !sameConfiguredRoute(route, byID[route.CanonicalModelID]) || row.ID != route.CanonicalModelID && !slices.Contains(route.Aliases, row.ID) {
			return false
		}
		if row.ID == route.CanonicalModelID {
			for _, alias := range route.Aliases {
				if !sameConfiguredRoute(route, byID[alias]) {
					return false
				}
			}
		}
	}
	return true
}
