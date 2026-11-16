package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const skillsCatalogYAML = `providers:
  anthropic-primary:
    type: anthropic
    apiKeyEnv: ANTHROPIC_API_KEY
  anthropic-backup:
    type: anthropic
    apiKeyEnv: ANTHROPIC_API_KEY
  openai-primary:
    type: openai
    apiKeyEnv: OPENAI_API_KEY
  compatible-primary:
    type: openai-compatible
    apiKeyEnv: COMPATIBLE_API_KEY
    baseURL: https://backend.invalid/v1
models:
  grafana/assistant:
    name: Grafana Assistant
    primary:
      provider: anthropic-primary
      model: claude-test
PRIMARY_SKILLS
    fallback:
      - provider: anthropic-backup
        model: claude-test
FALLBACK_SKILLS
`

func skillsCatalog(primary, fallback string) string {
	return strings.NewReplacer("PRIMARY_SKILLS\n", primary, "FALLBACK_SKILLS\n", fallback).Replace(skillsCatalogYAML)
}

func skillsBlock(indent, allow string) string {
	lines := strings.Split(strings.TrimSuffix(allow, "\n"), "\n")
	var b strings.Builder
	b.WriteString(indent + "skills:\n")
	b.WriteString(indent + "  allow:\n")
	for _, line := range lines {
		b.WriteString(indent + "    " + line + "\n")
	}
	return b.String()
}

func primarySkills(allow string) string  { return skillsBlock("      ", allow) }
func fallbackSkills(allow string) string { return skillsBlock("        ", allow) }

func TestParseModelCatalog_CandidateSkillsPolicy(t *testing.T) {
	t.Run("valid primary and fallback policies", func(t *testing.T) {
		file, err := ParseModelCatalog(skillsCatalog(
			primarySkills("- type: anthropic\n  skillId: pptx\n  versions: [latest, \"20251013\"]\n- type: custom\n  skillId: skill_01abc"),
			fallbackSkills("- type: anthropic\n  skillId: xlsx"),
		))
		require.NoError(t, err)
		model := file.Models["grafana/assistant"]
		assert.Equal(t, &SkillsPolicy{Allow: []AllowedSkill{
			{Type: SkillAnthropic, SkillID: "pptx", Versions: []string{"latest", "20251013"}},
			{Type: SkillCustom, SkillID: "skill_01abc"},
		}}, model.Primary.Skills)
		require.Len(t, model.Fallback, 1)
		assert.Equal(t, &SkillsPolicy{Allow: []AllowedSkill{{Type: SkillAnthropic, SkillID: "xlsx"}}}, model.Fallback[0].Skills)
	})

	t.Run("no policy", func(t *testing.T) {
		file, err := ParseModelCatalog(skillsCatalog("", ""))
		require.NoError(t, err)
		assert.Nil(t, file.Models["grafana/assistant"].Primary.Skills)
	})

	pptx := "- type: anthropic\n  skillId: pptx"
	for _, tc := range []struct {
		name            string
		primaryProvider string
		primary         string
		fallback        string
		wantErr         string
	}{
		{name: "openai candidate", primaryProvider: "openai-primary", primary: primarySkills(pptx), wantErr: "config: models.grafana/assistant.primary.skills is only supported for anthropic providers"},
		{name: "empty allow", primary: "      skills:\n        allow: []\n", wantErr: "config: models.grafana/assistant.primary.skills.allow must not be empty"},
		{name: "missing allow", primary: "      skills: {}\n", wantErr: "config: models.grafana/assistant.primary.skills.allow must not be empty"},
		{name: "unknown type", primary: primarySkills("- type: workspace\n  skillId: pptx"), wantErr: "config: models.grafana/assistant.primary.skills.allow[0].type is unsupported (want anthropic or custom)"},
		{name: "repeated entry", fallback: fallbackSkills(pptx + "\n" + pptx), wantErr: "config: models.grafana/assistant.fallback[0].skills.allow[1] repeats a skill"},
		{name: "empty skill ID", primary: primarySkills("- type: anthropic\n  skillId: \"\""), wantErr: "config: models.grafana/assistant.primary.skills.allow[0].skillId must be non-blank UTF-8 of at most 128 bytes"},
		{name: "blank skill ID", primary: primarySkills("- type: anthropic\n  skillId: \"  \""), wantErr: "config: models.grafana/assistant.primary.skills.allow[0].skillId must be non-blank UTF-8 of at most 128 bytes"},
		{name: "over-long skill ID", primary: primarySkills("- type: anthropic\n  skillId: " + strings.Repeat("s", 129)), wantErr: "config: models.grafana/assistant.primary.skills.allow[0].skillId must be non-blank UTF-8 of at most 128 bytes"},
		{name: "empty versions", primary: primarySkills(pptx + "\n  versions: []"), wantErr: "config: models.grafana/assistant.primary.skills.allow[0].versions must not be empty"},
		{name: "blank version", primary: primarySkills(pptx + "\n  versions: [\" \"]"), wantErr: "config: models.grafana/assistant.primary.skills.allow[0].versions[0] must be non-blank UTF-8 of at most 128 bytes"},
		{name: "over-long version", primary: primarySkills(pptx + "\n  versions: [" + strings.Repeat("v", 129) + "]"), wantErr: "config: models.grafana/assistant.primary.skills.allow[0].versions[0] must be non-blank UTF-8 of at most 128 bytes"},
		{name: "repeated version", primary: primarySkills(pptx + "\n  versions: [latest, latest]"), wantErr: "config: models.grafana/assistant.primary.skills.allow[0].versions[1] repeats a version"},
		{name: "unknown field", primary: primarySkills(pptx + "\n  scope: all"), wantErr: "config: invalid YAML fields or values"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			document := skillsCatalog(tc.primary, tc.fallback)
			if tc.primaryProvider != "" {
				document = strings.Replace(document, "      provider: anthropic-primary\n", "      provider: "+tc.primaryProvider+"\n", 1)
			}
			_, err := ParseModelCatalog(document)
			require.EqualError(t, err, tc.wantErr)
			assert.NotContains(t, err.Error(), "pptx")
		})
	}

	t.Run("openai-compatible candidate", func(t *testing.T) {
		document := strings.Replace(skillsCatalog("", fallbackSkills(pptx)), "      - provider: anthropic-backup\n", "      - provider: compatible-primary\n", 1)
		_, err := ParseModelCatalog(document)
		require.EqualError(t, err, "config: models.grafana/assistant.fallback[0].skills is only supported for anthropic providers")
	})

	t.Run("candidates differing only by policy repeat", func(t *testing.T) {
		document := skillsCatalog("", fallbackSkills(pptx))
		document = strings.Replace(document, "      - provider: anthropic-backup\n        model: claude-test\n", "      - provider: anthropic-backup\n        model: claude-test\n      - provider: anthropic-backup\n        model: claude-test\n", 1)
		_, err := ParseModelCatalog(document)
		require.EqualError(t, err, "config: models.grafana/assistant.fallback[1] repeats a candidate")
	})

	t.Run("invalid UTF-8", func(t *testing.T) {
		for name, policy := range map[string]*SkillsPolicy{
			"skill ID": {Allow: []AllowedSkill{{Type: SkillAnthropic, SkillID: "\xff"}}},
			"version":  {Allow: []AllowedSkill{{Type: SkillAnthropic, SkillID: "pptx", Versions: []string{"\xff"}}}},
		} {
			t.Run(name, func(t *testing.T) {
				file := testModelConfig()
				model := file.Models["canonical"]
				model.Primary.Skills = policy
				file.Models["canonical"] = model
				require.ErrorContains(t, file.Validate(), "must be non-blank UTF-8 of at most 128 bytes")
			})
		}
	})
}

const sharedDestinationYAML = `providers:
  anthropic-main:
    type: anthropic
    apiKeyEnv: ANTHROPIC_API_KEY
  anthropic-skills:
    type: anthropic
    apiKeyEnv: ANTHROPIC_API_KEY
  anthropic-other:
    type: anthropic
    apiKeyEnv: ANTHROPIC_API_KEY
models:
  grafana/a:
    name: A
    primary:
      provider: A_PROVIDER
      model: claude-x
A_SKILLS
  grafana/b:
    name: B
    primary:
      provider: anthropic-other
      model: claude-y
    fallback:
      - provider: B_PROVIDER
        model: claude-x
B_SKILLS
`

func TestParseModelCatalog_SkillsPolicyPerDestination(t *testing.T) {
	document := func(aProvider, aSkills, bProvider, bSkills string) string {
		return strings.NewReplacer("A_PROVIDER", aProvider, "A_SKILLS\n", aSkills, "B_PROVIDER", bProvider, "B_SKILLS\n", bSkills).Replace(sharedDestinationYAML)
	}
	pptx := "- type: anthropic\n  skillId: pptx\n  versions: [latest, \"1\"]\n- type: custom\n  skillId: skill_01abc"
	reordered := "- type: custom\n  skillId: skill_01abc\n- type: anthropic\n  skillId: pptx\n  versions: [\"1\", latest]"
	xlsx := "- type: anthropic\n  skillId: xlsx"
	const differs = "config: models.grafana/b.fallback[0].skills differs from models.grafana/a.primary.skills for the same provider and model"

	for _, tc := range []struct {
		name    string
		aSkills string
		bSkills string
	}{
		{name: "only the first declares skills", aSkills: primarySkills(pptx)},
		{name: "only the second declares skills", bSkills: fallbackSkills(pptx)},
		{name: "different allow lists", aSkills: primarySkills(pptx), bSkills: fallbackSkills(xlsx)},
		{name: "versions differ from no versions", aSkills: primarySkills("- type: anthropic\n  skillId: pptx"), bSkills: fallbackSkills("- type: anthropic\n  skillId: pptx\n  versions: [latest]")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseModelCatalog(document("anthropic-main", tc.aSkills, "anthropic-main", tc.bSkills))
			require.EqualError(t, err, differs)
			assert.NotContains(t, err.Error(), "pptx")
			assert.NotContains(t, err.Error(), "xlsx")
		})
	}

	t.Run("primary and primary", func(t *testing.T) {
		doc := strings.Replace(document("anthropic-main", primarySkills(pptx), "anthropic-main", ""), "    primary:\n      provider: anthropic-other\n      model: claude-y\n    fallback:\n      - provider: anthropic-main\n        model: claude-x\n", "    primary:\n      provider: anthropic-main\n      model: claude-x\n", 1)
		_, err := ParseModelCatalog(doc)
		require.EqualError(t, err, "config: models.grafana/b.primary.skills differs from models.grafana/a.primary.skills for the same provider and model")
	})

	t.Run("equal policies in a different order load", func(t *testing.T) {
		_, err := ParseModelCatalog(document("anthropic-main", primarySkills(pptx), "anthropic-main", fallbackSkills(reordered)))
		require.NoError(t, err)
	})

	t.Run("second provider instance may differ", func(t *testing.T) {
		_, err := ParseModelCatalog(document("anthropic-main", "", "anthropic-skills", fallbackSkills(pptx)))
		require.NoError(t, err)
	})
}
