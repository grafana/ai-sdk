package anthropicmessages

import "errors"

var (
	errInvalidRequest     = errors.New("invalid request")
	errUnsupportedRequest = errors.New("unsupported request")
)
