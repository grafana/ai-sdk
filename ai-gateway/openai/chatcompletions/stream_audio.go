package chatcompletions

import compatible "github.com/grafana/ai-sdk/providers/openai-compatible"

// Keep only identity and presence, never a second copy of the audio payload.
type audioStreamState struct {
	seen                              bool
	id                                string
	expiry                            int64
	hasExpiry, hasData, hasTranscript bool
}

func (s *audioStreamState) consume(a *compatible.ChatCompletionAudio) error {
	if a == nil {
		return nil
	}
	if err := a.Validate(); err != nil {
		return errOutput
	}
	if a.ID != nil && s.id != "" && s.id != *a.ID {
		return errOutput
	}
	if a.ExpiresAt != nil && s.hasExpiry && s.expiry != *a.ExpiresAt {
		return errOutput
	}
	s.seen = true
	if a.ID != nil {
		s.id = *a.ID
	}
	if a.ExpiresAt != nil {
		s.expiry = *a.ExpiresAt
		s.hasExpiry = true
	}
	s.hasData = s.hasData || a.Data != nil
	s.hasTranscript = s.hasTranscript || a.Transcript != nil
	return nil
}

func (s audioStreamState) complete() bool {
	return !s.seen || (s.id != "" && s.hasExpiry && s.hasData && s.hasTranscript)
}
