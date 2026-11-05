package provider

// CheckUnsupportedWarnings applies the caller's explicit preflight policy to
// native request-build warnings. Call it before sending any provider request.
// Warning details are deliberately excluded from the error: they may contain
// caller data, and the stable unsupported error is sufficient for classification.
func (o CallOptions) CheckUnsupportedWarnings(warnings []Warning) error {
	if o.RejectUnsupported {
		for _, warning := range warnings {
			if warning.Type == WarnUnsupported {
				return ErrUnsupportedFunctionality
			}
		}
	}
	return nil
}
