package chatcompletions

func (r mappedRequest) validateAudioRequest() error {
	for _, modality := range r.Modalities {
		if modality == "audio" && r.Audio == nil {
			return errRequest
		}
	}
	for _, message := range r.Messages {
		if message.Audio != nil && message.Role != roleAssistant {
			return errRequest
		}
	}
	return nil
}

func (r mappedRequest) needsNativeAudio() bool {
	if r.Audio != nil {
		return true
	}
	for _, modality := range r.Modalities {
		if modality == "audio" {
			return true
		}
	}
	for _, message := range r.Messages {
		if message.Audio != nil {
			return true
		}
	}
	return false
}
