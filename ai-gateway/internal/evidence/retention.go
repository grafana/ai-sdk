package evidence

import "encoding/json"

func (c *Component) encodedSize() (int, error) {
	if c.encodedBytes != 0 {
		return c.encodedBytes, nil
	}
	encoded, err := json.Marshal(c)
	return len(encoded), err
}

func (s *State) retainErrorLocked(attempt *Attempt, value *NativeError) {
	if value == nil {
		return
	}
	if value.Details != nil && value.Details.State == Available {
		remaining := SuccessDetailBytes
		for i := range s.attempts {
			other := &s.attempts[i]
			if other == attempt || other.NativeError == nil || other.NativeError.Details == nil {
				continue
			}
			size, err := other.NativeError.Details.encodedSize()
			if err != nil {
				s.invalid = true
				return
			}
			remaining -= size
		}
		size, err := value.Details.encodedSize()
		if err != nil {
			s.invalid = true
			return
		}
		if size > remaining {
			value.Details = &Component{State: OverLimit, Reason: AggregateLimit}
		}
	}
	attempt.NativeError = value
	s.enforceEssentialLocked()
}
