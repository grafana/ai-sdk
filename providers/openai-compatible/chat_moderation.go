package openaicompatible

import (
	"errors"
	"math"
	"unicode/utf8"
)

// ChatModerationOptions requests native inline moderation. No moderation is
// performed locally; the model and policies are enforced by the native service.
type ChatModerationOptions struct {
	Model  string                `json:"model"`
	Policy *ChatModerationPolicy `json:"policy,omitempty"`
}

// ChatModerationPolicy controls native input and output moderation.
type ChatModerationPolicy struct {
	Input  *ChatModerationMode `json:"input,omitempty"`
	Output *ChatModerationMode `json:"output,omitempty"`
}

// ChatModerationMode is either score-only or blocking.
type ChatModerationMode struct {
	Mode string `json:"mode"`
}

// Validate checks native moderation configuration.
func (m *ChatModerationOptions) Validate() error {
	if m == nil {
		return nil
	}
	if m.Model == "" || !utf8.ValidString(m.Model) {
		return errors.New("openai: invalid moderation model")
	}
	if m.Policy != nil {
		for _, p := range []*ChatModerationMode{m.Policy.Input, m.Policy.Output} {
			if p != nil && p.Mode != "score" && p.Mode != "block" {
				return errors.New("openai: invalid moderation policy")
			}
		}
	}
	return nil
}

// ChatModeration contains the two native moderation result channels.
type ChatModeration struct {
	Input  *ChatModerationOutcome `json:"input"`
	Output *ChatModerationOutcome `json:"output"`
}

// ChatModerationOutcome is a validated moderation_results or error union.
type ChatModerationOutcome struct {
	Type    string                 `json:"type"`
	Model   string                 `json:"model,omitempty"`
	Results []ChatModerationResult `json:"results,omitzero"`
	Code    *string                `json:"code,omitempty"`
	Message *string                `json:"message,omitempty"`
}

// ChatModerationResult preserves native categories and probability scores.
type ChatModerationResult struct {
	Type                      string              `json:"type"`
	Model                     string              `json:"model"`
	Flagged                   *bool               `json:"flagged"`
	Categories                map[string]bool     `json:"categories"`
	CategoryScores            map[string]float64  `json:"category_scores"`
	CategoryAppliedInputTypes map[string][]string `json:"category_applied_input_types"`
}

// Validate checks every published result and error field.
func (m *ChatModeration) Validate() error {
	if m == nil {
		return nil
	}
	invalid := errors.New("openai: invalid Chat moderation metadata")
	if m.Input == nil || m.Output == nil {
		return invalid
	}
	for _, o := range []*ChatModerationOutcome{m.Input, m.Output} {
		switch o.Type {
		case "error":
			if o.Code == nil || o.Message == nil || !utf8.ValidString(*o.Code) || !utf8.ValidString(*o.Message) || o.Model != "" || o.Results != nil {
				return invalid
			}
		case "moderation_results":
			if o.Model == "" || !utf8.ValidString(o.Model) || o.Results == nil || o.Code != nil || o.Message != nil {
				return invalid
			}
			for _, r := range o.Results {
				if r.Type != "moderation_result" || r.Model == "" || !utf8.ValidString(r.Model) || r.Flagged == nil || r.Categories == nil || r.CategoryScores == nil || r.CategoryAppliedInputTypes == nil {
					return invalid
				}
				for k := range r.Categories {
					if !utf8.ValidString(k) {
						return invalid
					}
				}
				for k, v := range r.CategoryScores {
					if !utf8.ValidString(k) || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
						return invalid
					}
				}
				for k, vs := range r.CategoryAppliedInputTypes {
					if !utf8.ValidString(k) || vs == nil {
						return invalid
					}
					for _, v := range vs {
						if v != "text" && v != "image" {
							return invalid
						}
					}
				}
			}
		default:
			return invalid
		}
	}
	return nil
}
