package nativeoptions

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func anthropicCall(options string) provider.CallOptions {
	return provider.CallOptions{ProviderOptions: provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(options)}}}
}

func typedCall(container anthropicprovider.Container) provider.CallOptions {
	return provider.CallOptions{ProviderOptions: provider.BuildProviderOptions(anthropicprovider.AnthropicOptions{Container: &container})}
}

func skillsCall(skills ...string) provider.CallOptions {
	return anthropicCall(`{"container":{"skills":[` + strings.Join(skills, ",") + `]}}`)
}

func distinctSkills(n int) []string {
	skills := make([]string, n)
	for i := range skills {
		skills[i] = fmt.Sprintf(`{"type":"anthropic","skillId":"skill-%d"}`, i)
	}
	return skills
}

func distinctGrants(n int) []AnthropicSkillGrant {
	grants := make([]AnthropicSkillGrant, n)
	for i := range grants {
		grants[i] = AnthropicSkillGrant{Type: anthropicprovider.ContainerSkillAnthropic, SkillID: fmt.Sprintf("skill-%d", i)}
	}
	return grants
}

const (
	pptxSkill   = `{"type":"anthropic","skillId":"pptx"}`
	customSkill = `{"type":"custom","providerReference":{"anthropic":"skill_01abc"}}`
)

func TestAnthropic_SkillsPolicy(t *testing.T) {
	configured := AnthropicPolicy{Skills: AllowlistedAnthropicSkills([]AnthropicSkillGrant{
		{Type: anthropicprovider.ContainerSkillAnthropic, SkillID: "pptx"},
		{Type: anthropicprovider.ContainerSkillCustom, SkillID: "skill_01abc"},
		{Type: anthropicprovider.ContainerSkillAnthropic, SkillID: "xlsx", Versions: []string{"20251013"}},
		{Type: anthropicprovider.ContainerSkillAnthropic, SkillID: "docx", Versions: []string{"latest"}},
	})}
	requestAccount := RequestAccountAnthropicPolicy()

	t.Run("configured admits", func(t *testing.T) {
		for name, call := range map[string]provider.CallOptions{
			"listed anthropic skill":           skillsCall(pptxSkill),
			"listed custom skill by reference": skillsCall(customSkill),
			"pinned version":                   skillsCall(`{"type":"anthropic","skillId":"xlsx","version":"20251013"}`),
			"empty version without versions":   skillsCall(`{"type":"anthropic","skillId":"pptx","version":""}`),
			"several listed skills":            skillsCall(pptxSkill, customSkill),
		} {
			t.Run(name, func(t *testing.T) {
				require.NoError(t, Anthropic(configured)(call))
			})
		}
	})

	t.Run("configured refuses", func(t *testing.T) {
		for name, call := range map[string]provider.CallOptions{
			"unlisted skill":                       skillsCall(`{"type":"anthropic","skillId":"pdf"}`),
			"listed ID under another type":         skillsCall(`{"type":"custom","providerReference":{"anthropic":"pptx"}}`),
			"wrong version":                        skillsCall(`{"type":"anthropic","skillId":"xlsx","version":"20250101"}`),
			"version against an unpinned entry":    skillsCall(`{"type":"anthropic","skillId":"pptx","version":"latest"}`),
			"omitted version against pinned":       skillsCall(`{"type":"anthropic","skillId":"xlsx"}`),
			"empty version against pinned":         skillsCall(`{"type":"anthropic","skillId":"xlsx","version":""}`),
			"omitted version against latest":       skillsCall(`{"type":"anthropic","skillId":"docx"}`),
			"one unlisted skill among listed ones": skillsCall(pptxSkill, `{"type":"anthropic","skillId":"pdf"}`),
		} {
			t.Run(name, func(t *testing.T) {
				require.ErrorIs(t, Anthropic(configured)(call), catalog.ErrUnsupportedRequest)
			})
		}
	})

	t.Run("no policy refuses every skill", func(t *testing.T) {
		for name, policy := range map[string]AnthropicPolicy{
			"zero policy":  {},
			"nil grants":   {Skills: AllowlistedAnthropicSkills(nil)},
			"empty grants": {Skills: AllowlistedAnthropicSkills([]AnthropicSkillGrant{})},
		} {
			t.Run(name, func(t *testing.T) {
				require.ErrorIs(t, Anthropic(policy)(skillsCall(pptxSkill)), catalog.ErrUnsupportedRequest)
				require.ErrorIs(t, Anthropic(policy)(skillsCall(customSkill)), catalog.ErrUnsupportedRequest)
			})
		}
	})

	t.Run("request account admits bounded skills", func(t *testing.T) {
		require.NoError(t, Anthropic(requestAccount)(skillsCall(pptxSkill, customSkill, `{"type":"anthropic","skillId":"pdf","version":"latest"}`)))
		require.NoError(t, Anthropic(requestAccount)(anthropicCall(`{"container":{"id":"container_01","skills":[`+customSkill+`]}}`)))
	})

	t.Run("skill count bound", func(t *testing.T) {
		require.NoError(t, Anthropic(requestAccount)(skillsCall(distinctSkills(maxAnthropicSkills)...)))
		require.NoError(t, Anthropic(AnthropicPolicy{Skills: AllowlistedAnthropicSkills(distinctGrants(maxAnthropicSkills))})(skillsCall(distinctSkills(maxAnthropicSkills)...)))
		for name, policy := range map[string]AnthropicPolicy{
			"request account": requestAccount,
			"configured":      {Skills: AllowlistedAnthropicSkills(distinctGrants(maxAnthropicSkills + 1))},
		} {
			t.Run(name, func(t *testing.T) {
				require.ErrorIs(t, Anthropic(policy)(skillsCall(distinctSkills(maxAnthropicSkills+1)...)), catalog.ErrUnsupportedRequest)
			})
		}
	})

	t.Run("static bounds in both modes", func(t *testing.T) {
		long := strings.Repeat("s", maxAnthropicSkillBytes+1)
		permissive := AnthropicPolicy{Skills: AllowlistedAnthropicSkills([]AnthropicSkillGrant{
			{Type: anthropicprovider.ContainerSkillAnthropic, SkillID: "pptx", Versions: []string{strings.Repeat("v", maxAnthropicSkillBytes+1), "\xff"}},
			{Type: anthropicprovider.ContainerSkillAnthropic, SkillID: long},
			{Type: anthropicprovider.ContainerSkillAnthropic, SkillID: "\xff"},
			{Type: anthropicprovider.ContainerSkillCustom, SkillID: long},
			{Type: anthropicprovider.ContainerSkillCustom, SkillID: "external"},
		})}
		for name, call := range map[string]provider.CallOptions{
			"unknown type":                skillsCall(`{"type":"workspace","skillId":"pptx"}`),
			"empty anthropic ID":          skillsCall(`{"type":"anthropic","skillId":""}`),
			"over-long anthropic ID":      skillsCall(`{"type":"anthropic","skillId":"` + long + `"}`),
			"over-long custom ID":         skillsCall(`{"type":"custom","providerReference":{"anthropic":"` + long + `"}}`),
			"invalid UTF-8 ID":            typedCall(anthropicprovider.Container{Skills: []anthropicprovider.ContainerSkill{{Type: anthropicprovider.ContainerSkillAnthropic, SkillID: "\xff"}}}),
			"over-long version":           skillsCall(`{"type":"anthropic","skillId":"pptx","version":"` + strings.Repeat("v", maxAnthropicSkillBytes+1) + `"}`),
			"invalid UTF-8 version":       typedCall(anthropicprovider.Container{Skills: []anthropicprovider.ContainerSkill{{Type: anthropicprovider.ContainerSkillAnthropic, SkillID: "pptx", Version: "\xff"}}}),
			"duplicate skill":             skillsCall(pptxSkill, pptxSkill),
			"duplicate custom skill":      skillsCall(customSkill, customSkill),
			"reference without anthropic": skillsCall(`{"type":"custom","providerReference":{"openai":"external"}}`),
			"empty anthropic reference":   skillsCall(`{"type":"custom","providerReference":{"anthropic":""}}`),
			"null anthropic reference":    skillsCall(`{"type":"custom","providerReference":{"anthropic":null}}`),
			"custom skill by skill ID":    skillsCall(`{"type":"custom","skillId":"external"}`),
			"skills that are not a list":  anthropicCall(`{"container":{"skills":{"type":"anthropic"}}}`),
		} {
			t.Run(name, func(t *testing.T) {
				for mode, policy := range map[string]AnthropicPolicy{"request account": requestAccount, "configured": permissive} {
					assert.ErrorIs(t, Anthropic(policy)(call), catalog.ErrUnsupportedRequest, mode)
				}
			})
		}
	})

	t.Run("absent skills are unchanged", func(t *testing.T) {
		for name, call := range map[string]provider.CallOptions{
			"no options":       {},
			"null container":   anthropicCall(`{"container":null}`),
			"empty skills":     anthropicCall(`{"container":{"skills":[]}}`),
			"null skills":      anthropicCall(`{"container":{"skills":null}}`),
			"unrelated option": anthropicCall(`{"sendReasoning":true}`),
		} {
			t.Run(name, func(t *testing.T) {
				for _, policy := range []AnthropicPolicy{{}, requestAccount} {
					require.NoError(t, Anthropic(policy)(call))
				}
			})
		}
	})

	t.Run("MCP checks are unchanged", func(t *testing.T) {
		call := anthropicCall(`{"mcpServers":[{"name":"echo","url":"http://mcp.example.test"}],"container":{"skills":[` + pptxSkill + `]}}`)
		require.ErrorIs(t, Anthropic(configured)(call), catalog.ErrUnsupportedRequest)
		require.ErrorIs(t, Anthropic(requestAccount)(call), catalog.ErrUnsupportedRequest)
	})

	t.Run("native fallbacks stay refused", func(t *testing.T) {
		require.ErrorIs(t, Anthropic(requestAccount)(anthropicCall(`{"fallbacks":"default"}`)), catalog.ErrUnsupportedRequest)
	})
}

func TestAnthropic_ContainerID(t *testing.T) {
	configured := AnthropicPolicy{Skills: AllowlistedAnthropicSkills([]AnthropicSkillGrant{{Type: anthropicprovider.ContainerSkillAnthropic, SkillID: "pptx"}})}
	requestAccount := RequestAccountAnthropicPolicy()
	exact := strings.Repeat("c", maxAnthropicContainerIDBytes)

	t.Run("configured refuses every container ID", func(t *testing.T) {
		for name, call := range map[string]provider.CallOptions{
			"ID only":                      anthropicCall(`{"container":{"id":"container_01"}}`),
			"ID with allowed skills":       anthropicCall(`{"container":{"id":"container_01","skills":[` + pptxSkill + `]}}`),
			"ID with an empty skills list": anthropicCall(`{"container":{"id":"container_01","skills":[]}}`),
		} {
			t.Run(name, func(t *testing.T) {
				for _, policy := range []AnthropicPolicy{{}, configured} {
					require.ErrorIs(t, Anthropic(policy)(call), catalog.ErrUnsupportedRequest)
				}
			})
		}
	})

	t.Run("configured admits an empty or absent container ID", func(t *testing.T) {
		for name, call := range map[string]provider.CallOptions{
			"empty ID":                     anthropicCall(`{"container":{"id":""}}`),
			"null container":               anthropicCall(`{"container":null}`),
			"empty ID with allowed skills": anthropicCall(`{"container":{"id":"","skills":[` + pptxSkill + `]}}`),
		} {
			t.Run(name, func(t *testing.T) {
				require.NoError(t, Anthropic(configured)(call))
			})
		}
	})

	t.Run("request account admits a bounded container ID", func(t *testing.T) {
		require.NoError(t, Anthropic(requestAccount)(anthropicCall(`{"container":{"id":"`+exact+`"}}`)))
		require.NoError(t, Anthropic(requestAccount)(anthropicCall(`{"container":{"id":"container_01","skills":[`+customSkill+`]}}`)))
	})

	t.Run("request account refuses an unbounded container ID", func(t *testing.T) {
		for name, call := range map[string]provider.CallOptions{
			"over-long":     anthropicCall(`{"container":{"id":"` + exact + `c"}}`),
			"invalid UTF-8": typedCall(anthropicprovider.Container{ID: "\xff"}),
		} {
			t.Run(name, func(t *testing.T) {
				require.ErrorIs(t, Anthropic(requestAccount)(call), catalog.ErrUnsupportedRequest)
			})
		}
	})

	t.Run("refusals do not echo the ID", func(t *testing.T) {
		err := Anthropic(AnthropicPolicy{})(anthropicCall(`{"container":{"id":"container_secret"}}`))
		require.ErrorIs(t, err, catalog.ErrUnsupportedRequest)
		assert.NotContains(t, err.Error(), "container_secret")
	})
}
