package nativeoptions

import (
	"slices"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
)

const (
	// maxAnthropicSkills is a Gateway resource bound set to Anthropic's
	// documented per-request limit ("Using Agent Skills with the API",
	// Limits and constraints, Request limits: maximum Skills for each
	// request: 20). Upstream @ai-sdk/anthropic sets no limit.
	maxAnthropicSkills           = 20
	maxAnthropicSkillBytes       = 128
	maxAnthropicContainerIDBytes = 128
)

// AnthropicPolicy controls which Anthropic container options one candidate
// admits. The zero value refuses every container skill and every container ID.
type AnthropicPolicy struct {
	Skills       AnthropicSkillsPolicy
	containerIDs containerIDMode
}

type containerIDMode int

const (
	containerIDsRefused containerIDMode = iota
	containerIDsBounded
)

type anthropicSkillsMode int

const (
	anthropicSkillsDenied anthropicSkillsMode = iota
	anthropicSkillsBounded
	anthropicSkillsAllowlisted
)

// AnthropicSkillsPolicy decides which container skills a candidate admits.
// The zero value refuses every non-empty skills list.
type AnthropicSkillsPolicy struct {
	mode  anthropicSkillsMode
	allow map[skillKey][]string
}

// AnthropicSkillGrant admits one skill by type and resolved skill ID. Without
// Versions it admits only requests that omit the version; otherwise the request
// version must be listed.
type AnthropicSkillGrant struct {
	Type     anthropicprovider.ContainerSkillType
	SkillID  string
	Versions []string
}

type skillKey struct {
	skillType anthropicprovider.ContainerSkillType
	skillID   string
}

// AllowlistedAnthropicSkills returns a configured-account skills policy that
// admits only the granted skills within the static bounds. Nil or empty grants
// return the zero policy, which refuses every skill.
func AllowlistedAnthropicSkills(grants []AnthropicSkillGrant) AnthropicSkillsPolicy {
	if len(grants) == 0 {
		return AnthropicSkillsPolicy{}
	}
	allow := make(map[skillKey][]string, len(grants))
	for _, grant := range grants {
		allow[skillKey{skillType: grant.Type, skillID: grant.SkillID}] = slices.Clone(grant.Versions)
	}
	return AnthropicSkillsPolicy{mode: anthropicSkillsAllowlisted, allow: allow}
}

// RequestAccountAnthropicPolicy returns the policy for request-supplied (BYOK)
// accounts: skills and container IDs belong to the caller's account, so they
// are admitted within the static bounds.
func RequestAccountAnthropicPolicy() AnthropicPolicy {
	return AnthropicPolicy{
		Skills:       AnthropicSkillsPolicy{mode: anthropicSkillsBounded},
		containerIDs: containerIDsBounded,
	}
}

func (policy AnthropicPolicy) validateContainer(container *anthropicprovider.Container) error {
	if container == nil {
		return nil
	}
	if container.ID != "" {
		if policy.containerIDs != containerIDsBounded || !validAnthropicText(container.ID, maxAnthropicContainerIDBytes) {
			return catalog.ErrUnsupportedRequest
		}
	}
	if len(container.Skills) == 0 {
		return nil
	}
	return policy.Skills.validate(container.Skills)
}

func (policy AnthropicSkillsPolicy) validate(skills []anthropicprovider.ContainerSkill) error {
	if policy.mode == anthropicSkillsDenied || len(skills) > maxAnthropicSkills {
		return catalog.ErrUnsupportedRequest
	}
	seen := make(map[skillKey]struct{}, len(skills))
	for _, skill := range skills {
		id, err := skill.ResolveSkillID()
		if err != nil || !validAnthropicText(id, maxAnthropicSkillBytes) {
			return catalog.ErrUnsupportedRequest
		}
		if skill.Version != "" && !validAnthropicText(skill.Version, maxAnthropicSkillBytes) {
			return catalog.ErrUnsupportedRequest
		}
		key := skillKey{skillType: skill.Type, skillID: id}
		if _, duplicate := seen[key]; duplicate {
			return catalog.ErrUnsupportedRequest
		}
		seen[key] = struct{}{}
		if policy.mode == anthropicSkillsAllowlisted && !policy.allows(key, skill.Version) {
			return catalog.ErrUnsupportedRequest
		}
	}
	return nil
}

func (policy AnthropicSkillsPolicy) allows(key skillKey, version string) bool {
	versions, ok := policy.allow[key]
	if !ok {
		return false
	}
	if versions == nil {
		return version == ""
	}
	return slices.Contains(versions, version)
}

func validAnthropicText(value string, maxBytes int) bool {
	return value != "" && len(value) <= maxBytes && utf8.ValidString(value)
}
