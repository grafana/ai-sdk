package chatcompletions

import "github.com/grafana/ai-sdk/provider"

// nativeDelta is shared by single and indexed streams so retained native fields
// have the same request policy and projection in either protocol shape.
func (s *streamState) nativeDelta(r mappedRequest, metadata provider.ProviderMetadata) (delta, any, error) {
	native, err := chatMetadata(metadata)
	if err != nil {
		return delta{}, nil, err
	}
	probabilities, err := r.projectLogprobs(metadata, native.Logprobs)
	if err != nil {
		return delta{}, nil, err
	}
	if native.Refusal != nil {
		s.refused = true
	}
	return delta{Refusal: native.Refusal, Audio: native.Audio, Annotations: nativeAnnotations(native.Annotations)}, probabilities, nil
}
