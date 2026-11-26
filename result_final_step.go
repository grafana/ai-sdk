package aisdk

// FinalStep waits for the stream to finish and returns the last recorded step
// by value. It does not drain the stream; consume FullStream or its UI
// conversion before calling FinalStep, or drain it concurrently. Do not call
// FinalStep from a synchronous callback on the same run: the stream cannot
// finish until that callback returns.
//
// If no step was recorded, FinalStep returns a zero StepResult and false. If
// the stream fails or is canceled after recording a step, FinalStep still
// returns the last recorded step and true. The bool does not indicate success.
// As with Steps, nested data is shared and must be treated as read-only. Copy
// any nested data you need to modify. Do not modify step history while reading
// the result concurrently.
func (r *StreamTextResult) FinalStep() (StepResult, bool) {
	if r.done == nil {
		return StepResult{}, false
	}
	return finalRecordedStep(r.Steps())
}

// FinalStep returns the last entry of Steps by value, or a zero StepResult
// and false if Steps is empty. Nested data is shared and must be treated as
// read-only. Copy any nested data you need to modify. Do not modify Steps while
// reading the result concurrently.
func (r *GenerateTextResult) FinalStep() (StepResult, bool) {
	return finalRecordedStep(r.Steps)
}

func finalRecordedStep(steps []StepResult) (StepResult, bool) {
	if len(steps) == 0 {
		return StepResult{}, false
	}
	return steps[len(steps)-1], true
}
