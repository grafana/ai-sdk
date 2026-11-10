package execution

import "github.com/grafana/ai-sdk/provider"

func validateChoices(result *provider.GenerateResult, history ToolHistory, count int) error {
	if result == nil || count > 128 || len(result.Choices) != count || len(result.Content) != 0 || result.FinishReason.Unified != "" {
		return ErrInvalidOutput
	}
	seen := make([]bool, count)
	for _, choice := range result.Choices {
		if choice.Index < 0 || choice.Index >= count || seen[choice.Index] {
			return ErrInvalidOutput
		}
		seen[choice.Index] = true
		if err := ValidateGenerateResult(&provider.GenerateResult{Content: choice.Content, FinishReason: choice.FinishReason}, history); err != nil {
			return err
		}
	}
	return nil
}

type outputValidator interface {
	Check(provider.StreamPart) error
	SegmentFinished() bool
	TrailingError() bool
}

type choicesValidator struct {
	choices                          []*StreamValidator
	indexed, finished, trailingError bool
}

func newOutputValidator(history ToolHistory, count int) outputValidator {
	if count <= 1 {
		return NewStreamValidator(history)
	}
	if count > 128 {
		return &choicesValidator{}
	}
	v := &choicesValidator{choices: make([]*StreamValidator, count)}
	for i := range v.choices {
		v.choices[i] = NewStreamValidator(history)
	}
	return v
}

func (v *choicesValidator) Check(part provider.StreamPart) error {
	if len(v.choices) == 0 {
		return ErrInvalidOutput
	}
	if v.finished && part.Type != provider.PartRaw && part.Type != provider.PartError {
		return ErrInvalidOutput
	}
	if part.ChoiceIndex != nil {
		index := *part.ChoiceIndex
		if index < 0 || index >= len(v.choices) || part.Type == provider.PartChoicesFinish || part.Type == provider.PartStreamStart || part.Type == provider.PartResponseMeta {
			return ErrInvalidOutput
		}
		part.ChoiceIndex = nil
		if v.choices[index].Check(part) != nil {
			return ErrInvalidOutput
		}
		v.indexed = true
	} else {
		switch part.Type {
		case provider.PartChoicesFinish:
			if part.Usage == nil || part.FinishReason != nil || len(part.Warnings) > 0 {
				return ErrInvalidOutput
			}
			for _, choice := range v.choices {
				if !choice.SegmentFinished() {
					return ErrInvalidOutput
				}
			}
			v.finished = true
		case provider.PartStreamStart, provider.PartResponseMeta:
			if v.indexed {
				return ErrInvalidOutput
			}
			for _, choice := range v.choices {
				if choice.Check(part) != nil {
					return ErrInvalidOutput
				}
			}
		case provider.PartRaw, provider.PartError:
			for _, choice := range v.choices {
				if choice.Check(part) != nil {
					return ErrInvalidOutput
				}
			}
		default:
			return ErrInvalidOutput
		}
	}
	if part.Type == provider.PartError {
		v.trailingError = true
	} else if part.Type != provider.PartRaw {
		v.trailingError = false
	}
	return nil
}
func (v *choicesValidator) SegmentFinished() bool { return v.finished }
func (v *choicesValidator) TrailingError() bool   { return v.trailingError }
