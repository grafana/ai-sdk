package config

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

const maxSkillPolicyBytes = 128

// SkillType identifies who owns an allowed Anthropic container skill.
type SkillType string

const (
	// SkillAnthropic is an Anthropic-provided skill, such as pptx.
	SkillAnthropic SkillType = "anthropic"
	// SkillCustom is a skill uploaded to the operator's Anthropic workspace.
	// Listing one shares it with every caller of the candidate.
	SkillCustom SkillType = "custom"
)

// SkillsPolicy lists the Anthropic container skills that callers of one
// configured candidate may request. A candidate without a policy refuses
// every skill. Only anthropic candidates may carry a policy, and every
// candidate that names the same provider instance and backend model must carry
// an equal policy.
type SkillsPolicy struct {
	Allow []AllowedSkill `yaml:"allow"`
}

// AllowedSkill admits one skill. SkillID is the Anthropic skill ID; for a
// custom skill it is the ID the request's providerReference.anthropic resolves
// to. Without Versions, only requests that omit the version are admitted;
// otherwise the request version must be listed. "latest" is matched literally.
type AllowedSkill struct {
	Type     SkillType `yaml:"type"`
	SkillID  string    `yaml:"skillId"`
	Versions []string  `yaml:"versions,omitempty"`
}

type skillIdentity struct {
	skillType SkillType
	skillID   string
}

func identityOf(entry AllowedSkill) skillIdentity {
	return skillIdentity{skillType: entry.Type, skillID: entry.SkillID}
}

type candidateKey struct {
	provider string
	model    string
}

func keyOf(candidate Primary) candidateKey {
	return candidateKey{provider: candidate.Provider, model: candidate.Model}
}

func validSkillPolicyText(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= maxSkillPolicyBytes && utf8.ValidString(value)
}

func validateSkillsPolicy(path string, policy *SkillsPolicy, providerType string) error {
	if policy == nil {
		return nil
	}
	if providerType != "anthropic" {
		return fmt.Errorf("config: %s.skills is only supported for anthropic providers", path)
	}
	if len(policy.Allow) == 0 {
		return fmt.Errorf("config: %s.skills.allow must not be empty", path)
	}
	seen := make(map[skillIdentity]struct{}, len(policy.Allow))
	for i, entry := range policy.Allow {
		entryPath := fmt.Sprintf("%s.skills.allow[%d]", path, i)
		if entry.Type != SkillAnthropic && entry.Type != SkillCustom {
			return fmt.Errorf("config: %s.type is unsupported (want anthropic or custom)", entryPath)
		}
		if !validSkillPolicyText(entry.SkillID) {
			return fmt.Errorf("config: %s.skillId must be non-blank UTF-8 of at most %d bytes", entryPath, maxSkillPolicyBytes)
		}
		if _, exists := seen[identityOf(entry)]; exists {
			return fmt.Errorf("config: %s repeats a skill", entryPath)
		}
		seen[identityOf(entry)] = struct{}{}
		if entry.Versions == nil {
			continue
		}
		if len(entry.Versions) == 0 {
			return fmt.Errorf("config: %s.versions must not be empty", entryPath)
		}
		versions := make(map[string]struct{}, len(entry.Versions))
		for j, version := range entry.Versions {
			if !validSkillPolicyText(version) {
				return fmt.Errorf("config: %s.versions[%d] must be non-blank UTF-8 of at most %d bytes", entryPath, j, maxSkillPolicyBytes)
			}
			if _, exists := versions[version]; exists {
				return fmt.Errorf("config: %s.versions[%d] repeats a version", entryPath, j)
			}
			versions[version] = struct{}{}
		}
	}
	return nil
}

// equalSkillsPolicies compares validated policies as sets of entries, each
// with its set of versions. A nil policy equals only another nil policy.
func equalSkillsPolicies(a, b *SkillsPolicy) bool {
	if a == nil || b == nil {
		return a == b
	}
	if len(a.Allow) != len(b.Allow) {
		return false
	}
	versions := make(map[skillIdentity][]string, len(a.Allow))
	for _, entry := range a.Allow {
		versions[identityOf(entry)] = entry.Versions
	}
	for _, entry := range b.Allow {
		other, ok := versions[identityOf(entry)]
		if !ok || (other == nil) != (entry.Versions == nil) || !sameVersions(other, entry.Versions) {
			return false
		}
	}
	return true
}

func sameVersions(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

// validateSkillsPolicyPerDestination requires one skills policy per provider
// instance and backend model across routes, because request routing attempts a
// repeated destination once and would otherwise apply whichever placement came
// first.
func (file File) validateSkillsPolicyPerDestination() error {
	type placement struct {
		path   string
		policy *SkillsPolicy
	}
	ids := make([]string, 0, len(file.Models))
	for id := range file.Models {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	first := make(map[candidateKey]placement)
	for _, id := range ids {
		model := file.Models[id]
		candidates := append([]Primary{model.Primary}, model.Fallback...)
		for index, candidate := range candidates {
			path := fmt.Sprintf("models.%s.primary", id)
			if index > 0 {
				path = fmt.Sprintf("models.%s.fallback[%d]", id, index-1)
			}
			key := keyOf(candidate)
			existing, ok := first[key]
			if !ok {
				first[key] = placement{path: path, policy: candidate.Skills}
				continue
			}
			if !equalSkillsPolicies(existing.policy, candidate.Skills) {
				return fmt.Errorf("config: %s.skills differs from %s.skills for the same provider and model", path, existing.path)
			}
		}
	}
	return nil
}
